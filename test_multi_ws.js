// Multi-turn test with SEPARATE WebSocket connections per turn
const WebSocket = require('/home/ts/HermesDeck/node_modules/ws');
const fs = require('fs');
const token = fs.readFileSync('/home/ts/.hermesdeck/server-token', 'utf8').trim();

function sendTurn(msg) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket('ws://127.0.0.1:28788/ws');
    let text = '';
    ws.on('open', () => {
      ws.send(JSON.stringify({type:'hello',protocolVersion:'1.0',clientName:'test',clientVersion:'1.0',token}));
    });
    ws.on('message', raw => {
      const m = JSON.parse(raw.toString());
      if (m.type === 'hello_ok') {
        ws.send(JSON.stringify({type:'request',id:'1',method:'submit_turn',params:{sessionKey:'ws-pool',channelKey:'web',message:msg}}));
      } else if (m.type === 'event') {
        const e = typeof m.event === 'string' ? JSON.parse(m.event) : m.event;
        if (e.type === 'assistant_text_delta') text += e.text;
        else if (e.type === 'turn_completed') {
          ws.close();
          resolve(text.trim());
        }
      }
    });
    ws.on('error', reject);
    setTimeout(() => reject(new Error('TO')), 30000);
  });
}

(async () => {
  console.log('=== T1 (独立连接) ===');
  const r1 = await sendTurn('我叫小明');
  console.log('R1:', JSON.stringify(r1));

  console.log('=== T2 (独立连接，同session) ===');
  const r2 = await sendTurn('我叫什么？');
  console.log('R2:', JSON.stringify(r2));

  console.log('=== T3 (独立连接) ===');
  const r3 = await sendTurn('帮我写个hello world');
  console.log('R3:', JSON.stringify(r3.slice(0,60)));

  console.log('=== DONE ===');
  process.exit(0);
})().catch(e => { console.log('ERR:', e.message); process.exit(1); });
