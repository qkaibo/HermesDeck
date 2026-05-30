// Direct gRPC multi-turn test (Node.js → Python sidecar, bypassing Gateway)
const grpc = require('/home/ts/HermesDeck/node_modules/@grpc/grpc-js');
const pl = require('/home/ts/HermesDeck/node_modules/@grpc/proto-loader');
const pkgDef = pl.loadSync('/home/ts/HermesDeck/proto/hermesdeck.proto',{keepCase:false,longs:String,enums:String,defaults:false,oneofs:true});
const hd = grpc.loadPackageDefinition(pkgDef).hermesdeck;
const client = new hd.HermesDeckBridge('localhost:29552', grpc.credentials.createInsecure());
let turn = 0;

function sendTurn(msg) {
  const call = client.ProcessMessage({sessionId:'node-ctx', userMessage:msg, stream:true});
  let text = '';
  call.on('data', r => { if(r.text) { text += r.text; process.stdout.write(r.text); } });
  call.on('end', () => {
    console.log(' [END turn=' + (++turn) + ' text=' + text.length + ']');
    if (turn === 1) sendTurn('我叫什么？');
    else process.exit(0);
  });
  call.on('error', e => { console.log('[ERR]', e.message.substring(0,100)); process.exit(1); });
}
console.log('=== Turn 1 ===');
sendTurn('我叫小明');
setTimeout(() => { console.log('[TO]'); process.exit(1); }, 40000);
