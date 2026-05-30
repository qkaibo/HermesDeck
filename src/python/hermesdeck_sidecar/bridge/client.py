"""Python → Go gRPC client for tool execution."""
from __future__ import annotations
import json, logging
from typing import Any, Dict, Optional
import grpc
from hermesdeck_sidecar.proto import hermesdeck_pb2 as pb
from hermesdeck_sidecar.proto import hermesdeck_pb2_grpc as pb_grpc

logger = logging.getLogger(__name__)


class BridgeClient:
    """Sends tool execution requests to Go Runtime."""

    def __init__(self, target: str = ""):
        self._target = target
        self._channel: Optional[grpc.aio.Channel] = None
        self._stub: Optional[pb_grpc.HermesDeckBridgeStub] = None

    @property
    def is_connected(self) -> bool:
        return self._stub is not None

    async def connect(self, target: str = ""):
        if target:
            self._target = target
        if not self._target:
            return
        try:
            self._channel = grpc.aio.insecure_channel(self._target)
            self._stub = pb_grpc.HermesDeckBridgeStub(self._channel)
            resp = await self._stub.HealthCheck(pb.HealthRequest(service="python"), timeout=5)
            logger.info("Connected to Go Runtime at %s (status=%s)", self._target, resp.status)
        except Exception as e:
            logger.warning("Cannot connect to Go Runtime: %s", e)
            self._channel = None
            self._stub = None

    async def execute_tool(
        self, tool_call_id: str, tool_name: str, arguments: Dict[str, Any]
    ) -> Dict[str, Any]:
        if not self._stub:
            return {"tool_call_id": tool_call_id, "is_error": True, "error_message": "Go Runtime not connected"}
        try:
            resp = await self._stub.ExecuteTool(
                pb.ToolRequest(
                    tool_call_id=tool_call_id, tool_name=tool_name,
                    arguments_json=json.dumps(arguments),
                ),
                timeout=60,
            )
            result = json.loads(resp.result_json) if resp.result_json else {}
            return {
                "tool_call_id": resp.tool_call_id, "result": result,
                "is_error": resp.is_error, "error_message": resp.error_message,
            }
        except Exception as e:
            logger.error("Tool execution error: %s", e)
            return {"tool_call_id": tool_call_id, "is_error": True, "error_message": str(e)}

    async def close(self):
        if self._channel:
            await self._channel.close()
            self._stub = None
            self._channel = None
