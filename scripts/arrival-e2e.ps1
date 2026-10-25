param(
    [Parameter(Mandatory=$true)][string]$BaseURL,
    [Parameter(Mandatory=$true)][string]$DriverToken,
    [Parameter(Mandatory=$true)][string]$PassengerToken,
    [Parameter(Mandatory=$true)][string]$DispatcherToken,
    [Parameter(Mandatory=$true)][string]$ReviewerToken,
    [Parameter(Mandatory=$true)][string]$DriverID,
    [Parameter(Mandatory=$true)][string]$PassengerID,
    [string]$OutputPath = 'arrival-evidence.json'
)
$ErrorActionPreference = 'Stop'
$base = $BaseURL.TrimEnd('/') + '/api/v1'
function Invoke-Arrival([string]$Method, [string]$Path, [string]$Token, [string]$Key, $Body) {
    $headers = @{ Authorization = 'Bearer ' + $Token }
    if ($Key) { $headers['Idempotency-Key'] = $Key }
    $args = @{ Method=$Method; Uri=$base+$Path; Headers=$headers; TimeoutSec=10 }
    if ($null -ne $Body) { $args['Body'] = ConvertTo-Json $Body -Depth 12 -Compress; $args['ContentType']='application/json; charset=utf-8' }
    try { return (Invoke-RestMethod @args).data }
    catch { throw ('Arrival HTTP call failed: {0} {1}; preserve the original command key for retry.' -f $Method,$Path) }
}
$evidence = @()
foreach ($mode in @('NORMAL','EXCEPTION')) {
    $id = 'demo-' + [guid]::NewGuid().ToString('N')
    $o1=$id+'-1'; $o2=$id+'-2'; $o3=$id+'-3'
    $pickup=@{latitude=31.2304;longitude=121.4737;crs='GCJ02'}
    $created=Invoke-Arrival POST '/ride-tasks' $DispatcherToken ($id+'-create') @{
        task_id=$id; pickup=$pickup; orders=@(@{order_id=$o1;passenger_id=$PassengerID},@{order_id=$o2;passenger_id=$PassengerID},@{order_id=$o3;passenger_id=$PassengerID})
    }
    $assigned=Invoke-Arrival POST "/ride-tasks/$id/assignments" $DispatcherToken ($id+'-assign') @{expected_task_version=$created.task_version;driver_id=$DriverID}
    $cancelled=Invoke-Arrival POST "/ride-orders/$o3/cancellations" $PassengerToken ($id+'-cancel') @{expected_task_version=$assigned.task_version;assignment_id=$assigned.assignment_id;assignment_version=$assigned.assignment_version}
    $body=@{expected_task_version=$cancelled.task_version;assignment_id=$assigned.assignment_id;assignment_version=$assigned.assignment_version;mode=$mode;evidence_ids=@()}
    if ($mode -eq 'NORMAL') {
        $sample=Invoke-Arrival POST "/ride-tasks/$id/location-samples" $DriverToken '' @{assignment_id=$assigned.assignment_id;assignment_version=$assigned.assignment_version;sample_id=$id+'-sample';sample_seq='1';coordinate=$pickup;collected_at=[DateTime]::UtcNow.AddSeconds(-1).ToString('o')}
        $body['sample_id']=$sample.sample_id
    } else { $body['exception_reason']='Location unavailable; driver reports arrival at the agreed pickup point.' }
    $key=$id+'-arrive'
    $arrival=Invoke-Arrival POST "/ride-tasks/$id/arrivals" $DriverToken $key $body
    $replay=Invoke-Arrival POST "/ride-tasks/$id/arrivals" $DriverToken $key $body
    if (($arrival | ConvertTo-Json -Depth 12 -Compress) -ne ($replay | ConvertTo-Json -Depth 12 -Compress)) { throw 'Idempotent replay changed the original result.' }
    if ($arrival.order_results.Count -ne 2) { throw 'Expected two arrived orders.' }
    $events=Invoke-Arrival GET "/ride-orders/$o1/events?limit=100" $PassengerToken '' $null
    $view=Invoke-Arrival GET "/ride-orders/$o1/arrival" $PassengerToken '' $null
    $cancelView=Invoke-Arrival GET "/ride-orders/$o3/arrival" $PassengerToken '' $null
    if ($events.items.Count -ne 1 -or $view.arrival.arrival_id -ne $arrival.arrival_id -or $null -ne $cancelView.arrival) { throw 'Query/event/cancelled-order assertions failed.' }
    $reviews=Invoke-Arrival GET '/arrival-reviews?limit=100' $ReviewerToken '' $null
    $reviewCount=@($reviews.items | Where-Object { $_.arrival_id -eq $arrival.arrival_id }).Count
    if (($mode -eq 'EXCEPTION' -and $reviewCount -ne 1) -or ($mode -eq 'NORMAL' -and $reviewCount -ne 0)) { throw 'Unexpected review count.' }
    $evidence+=@{mode=$mode;status=200;task_id=$id;arrival_id=$arrival.arrival_id;arrived_at=$arrival.arrived_at;order_results=$arrival.order_results;event_count=$events.items.Count;review_count=$reviewCount;replay_equal=$true;competition='not exercised by this script; see MongoDB integration tests'}
}
$evidence | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $OutputPath -Encoding UTF8
Write-Output ('Evidence saved to {0}; credentials omitted; fixture records retained.' -f $OutputPath)
