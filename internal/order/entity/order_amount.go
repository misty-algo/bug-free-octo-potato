package entity

// 金额口径（《技术方案 V2》3.1 判定顺序、计划附录 B）：
//
//	预估(estimated)：活动单取 priceSnap.F − cpFen − cdFen（估价快照，未出账单）；
//	应付(payable)：有账单未付取 P（bills[].payable）；
//	实付(paid)：账单 PAID 且无退款取 P；
//	退款后净实付(net_paid)：账单 PAID 有退款取 P − Σrefunds[].refundFee；
//	无账单且非活动单（如已取消免取消费）：0。
//
// 「未付不得显示成实付」：未付一律应付/预估。
const (
	AmountLabelEstimated = "预估"
	AmountLabelPayable   = "应付"
	AmountLabelPaid      = "实付"
	AmountLabelNetPaid   = "退款后净实付"

	AmountBasisEstimated = "estimated"
	AmountBasisPayable   = "payable"
	AmountBasisPaid      = "paid"
	AmountBasisNetPaid   = "net_paid"
)

// AmountSummary 是列表/详情卡片的金额口径出参。
type AmountSummary struct {
	Amount    int64  // 金额（分）
	Label     string // 预估/应付/实付/退款后净实付
	Basis     string // estimated/payable/paid/net_paid
	BillState string // 驱动金额口径的账单状态；无账单为空串
}

// AmountSummary 按口径计算订单卡片金额（判定顺序：主账单 → 取消费账单 → 活动单预估 → 0）。
func (o *Order) AmountSummary() AmountSummary {
	if bill := o.tripBill(); bill != nil {
		if bill.State == BillStatePaid {
			refunded := int64(0)
			// TODO(孙源)：退款写入 refunds[] 后校准成功态过滤；现按全量求和（净实付=P−Σr）。
			for _, refund := range o.Refunds {
				refunded += refund.RefundFee
			}
			if refunded > 0 {
				return AmountSummary{Amount: bill.PayableFee - refunded, Label: AmountLabelNetPaid, Basis: AmountBasisNetPaid, BillState: bill.State}
			}
			return AmountSummary{Amount: bill.PayableFee, Label: AmountLabelPaid, Basis: AmountBasisPaid, BillState: bill.State}
		}
		return AmountSummary{Amount: bill.PayableFee, Label: AmountLabelPayable, Basis: AmountBasisPayable, BillState: bill.State}
	}
	if bill := o.cancelFeeBill(); bill != nil {
		if bill.State == BillStatePaid {
			return AmountSummary{Amount: bill.PayableFee, Label: AmountLabelPaid, Basis: AmountBasisPaid, BillState: bill.State}
		}
		return AmountSummary{Amount: bill.PayableFee, Label: AmountLabelPayable, Basis: AmountBasisPayable, BillState: bill.State}
	}
	if IsOrderActive(o.State) {
		amount := int64(0)
		if o.PriceSnap != nil {
			amount = o.PriceSnap.F - o.CpFen - o.CdFen
			if amount < 0 {
				amount = 0
			}
		}
		return AmountSummary{Amount: amount, Label: AmountLabelEstimated, Basis: AmountBasisEstimated}
	}
	return AmountSummary{Amount: 0, Label: AmountLabelPayable, Basis: AmountBasisPayable}
}
