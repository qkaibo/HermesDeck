// Test gRPC response format from Node.js — with defaults: false
const grpc = require('/home/ts/HermesDeck/node_modules/@grpc/grpc-js');
const protoLoader = require('/home/ts/HermesDeck/node_modules/@grpc/proto-loader');
const path = require('path');

const pkgDef = protoLoader.loadSync('/home/ts/HermesDeck/proto/hermesdeck.proto', {
  keepCase: false, longs: String, enums: String, defaults: false, oneofs: true,
});
const hermesdeck = grpc.loadPackageDefinition(pkgDef).hermesdeck;
const client = new hermesdeck.HermesDeckBridge('localhost:29552', grpc.credentials.createInsecure());

console.log('Sending request...');
const call = client.ProcessMessage({
  session_id: 'node-test',
  user_message: 'hello, what model are you?',
  stream: true,
});

call.on('data', (resp) => {
  console.log('---');
  console.log('Keys:', Object.keys(resp));
  console.log('is_final:', resp.is_final);
  console.log('text:', JSON.stringify(resp.text));
  console.log('content:', JSON.stringify(resp.content));
  console.log('hasOwnProperty text:', resp.hasOwnProperty('text'));
  // Print full response
  console.log('full:', JSON.stringify(resp));
});
call.on('end', () => { console.log('---[END]---'); process.exit(0); });
call.on('error', (err) => { console.log('[ERR]', err.message, err.details); process.exit(1); });
setTimeout(() => { console.log('[TO]'); process.exit(1); }, 25000);
