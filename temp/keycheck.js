const fs = require('fs');
const crypto = require('crypto');
const text = fs.readFileSync('configs/config.yaml','utf8');
function grab(name, nextName){
  const re = new RegExp(name + ': \\|\n((?:\\s+.*\n)+?)\\s+' + nextName + ':');
  const m = text.match(re);
  if(!m) throw new Error('not found '+name);
  return m[1].split('\n').map(l=>l.trim()).join('');
}
const priv = '-----BEGIN RSA PRIVATE KEY-----\n'+grab('private_key','alipay_public_key')+'\n-----END RSA PRIVATE KEY-----';
const pub = '-----BEGIN PUBLIC KEY-----\n'+grab('alipay_public_key','production')+'\n-----END PUBLIC KEY-----';
const data = 'verify-key-pair';
const sig = crypto.createSign('RSA-SHA256').update(data).sign(priv,'base64');
const ok = crypto.createVerify('RSA-SHA256').update(data).verify(pub, sig, 'base64');
console.log('VERIFY_OK=' + ok);
