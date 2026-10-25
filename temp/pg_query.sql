\pset pager off
\d payments
SELECT id, order_no, amount, status, channel, idempotent_key FROM payments ORDER BY created_at DESC LIMIT 10;
