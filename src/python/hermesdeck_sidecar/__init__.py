"""HermesDeck Python Sidecar."""
from __future__ import annotations
import argparse, asyncio, logging, os
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="HermesDeck Python Sidecar")
    parser.add_argument("--port", type=int, default=29552)
    parser.add_argument("--go-addr", type=str, default="")
    parser.add_argument("--log-level", type=str, default="INFO")
    args = parser.parse_args()

    logging.basicConfig(
        level=getattr(logging, args.log_level.upper(), logging.INFO),
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    )
    if "HERMES_HOME" not in os.environ:
        os.environ["HERMES_HOME"] = str(Path(__file__).parent / "data")

    logger = logging.getLogger(__name__)
    logger.info("HermesDeck Python Sidecar starting...")

    from hermesdeck_sidecar.bridge.server import SidecarServer
    server = SidecarServer(port=args.port, go_addr=args.go_addr)

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    try:
        loop.run_until_complete(server.start())
    except KeyboardInterrupt:
        pass
    finally:
        loop.run_until_complete(server.stop())
        loop.close()


if __name__ == "__main__":
    main()
