"""Test the HermesDeck sidecar directly (no gRPC)."""
import asyncio, sys, logging
logging.basicConfig(level=logging.DEBUG)

sys.path.insert(0, "/home/ts/HermesDeck/src/python")

from hermesdeck_sidecar.hermes_adapter.agent_wrapper import HermesAgentWrapper

async def main():
    print("=== Creating wrapper ===")
    wrapper = HermesAgentWrapper()
    print("=== Initializing ===")
    await wrapper.initialize()
    print("=== INIT OK ===")
    
    print("=== Processing message ===")
    texts = []
    async for resp in wrapper.process("test1", "Say hi in 2 words"):
        t = resp.text if hasattr(resp, 'text') else str(resp)
        texts.append(t)
        sys.stdout.write(t)
        sys.stdout.flush()
    
    print()
    print("=== DONE ===")
    print(f"Total text fragments: {len(texts)}")
    print(f"Full response: {''.join(texts)}")

asyncio.run(main())
