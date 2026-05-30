"""Test multi-turn conversation through the sidecar directly."""
import asyncio, sys, logging
logging.basicConfig(level=logging.WARNING)
sys.path.insert(0, '/home/ts/HermesDeck/src/python')

from hermesdeck_sidecar.hermes_adapter.agent_wrapper import HermesAgentWrapper

async def main():
    w = HermesAgentWrapper()
    await w.initialize()
    
    print("=== Turn 1 ===")
    texts = []
    async for r in w.process("test-session", "我叫小明"):
        t = r.text if hasattr(r, 'text') else ''
        if t: texts.append(t)
    print(''.join(texts))
    
    print("=== Turn 2 ===")
    texts = []
    async for r in w.process("test-session", "我叫什么来着？"):
        t = r.text if hasattr(r, 'text') else ''
        if t: texts.append(t)
    response = ''.join(texts).strip()
    print(f"Response: '{response}'")
    if not response:
        print("[WARNING] Turn 2 produced empty response!")
    
    print("=== Turn 3 ===")
    texts = []
    async for r in w.process("test-session", "帮我写个hello world"):
        t = r.text if hasattr(r, 'text') else ''
        if t: texts.append(t)
    print(''.join(texts)[:100])
    
    await w.close()
    print("=== DONE ===")

asyncio.run(main())
