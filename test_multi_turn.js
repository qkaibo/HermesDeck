// Multi-turn test with full event logging
const WebSocket = require('/home/ts/HermesDeck/node_modules/ws');
const fs = require('fs');
const token = fs.readFileSync('/home/ts/.hermesdeck/server-token', 'utf8').trim();
const ws = new WebSocket('ws://127.0.0.1:28788/ws');
let turn = 0;

ws.on('open', () => {
  ws.send(JSON.stringify({type:'hello',protocolVersion:'1.0',clientName:'test',clientVersion:'1.0',token}));
});

ws.on('message', raw => {
  const m = JSON.parse(raw.toString());
  if (m.type === 'hello_ok') {
    console.log('=== Turn 1: "我叫小明" ===');
    ws.send(JSON.stringify({type:'request',id:'1',method:'submit_turn',params:{sessionKey:'ctx2',channelKey:'web',message:'我叫小明'}}));
  } else if (m.type === 'event') {
    const e = typeof m.event === 'string' ? JSON.parse(m.event) : m.event;
    const tp = e.type;
    if (tp === 'assistant_text_delta') process.stdout.write(e.text);
    else if (tp === 'tool_call_started') console.log('\n[TOOL:' + e.name + ']');
    else if (tp === 'tool_call_finished') console.log('[TOOL_OK:' + e.name + ']');
    else if (tp === 'turn_completed') {
      console.log('\n--- turn', turn+1, 'done ---');
      turn++;
      if (turn < 3) {
        const msgs = ['我叫什么？','帮我写个hello world'];
        console.log('=== Turn', turn+1, ': "' + msgs[turn-1] + '" ===');
        ws.send(JSON.stringify({type:'request',id:''+(turn+1),method:'submit_turn',params:{sessionKey:'ctx2',channelKey:'web',message:msgs[turn-1]}}));
      } else {
        ws.close();
      }
    }
  }
});
setTimeout(() => { console.log('[TO]'); process.exit(1); }, 120000);
