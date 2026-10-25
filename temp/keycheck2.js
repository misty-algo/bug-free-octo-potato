const fs = require('fs');
const crypto = require('crypto');
const text = fs.readFileSync('configs/config.yaml', 'utf8');
const m = text.match(/private_key: "([^"]+)"/);
const p2 = text.match(/alipay_public_key: "([^"]+)"/);
if (!m || !p2) { console.log('ERR: key fields not found'); process.exit(1); }
try {
  const priv = '-----BEGIN RSA PRIVATE KEY-----\n' + m[1] + '\n-----END RSA PRIVATE KEY-----';
  const pub = '-----BEGIN PUBLIC KEY-----\n' + p2[1] + '\n-----END PUBLIC KEY-----';
  const data = 'verify-key-pair';
  const sig = crypto.createSign('RSA-SHA256').update(data).sign(priv, 'base64');
  const ok = crypto.createVerify('RSA-SHA256').update(data).verify(pub, sig, 'base64');
  console.log('VERIFY_OK=' + ok);
} catch (e) {
  console.log('ERR: ' + e.message);
}
