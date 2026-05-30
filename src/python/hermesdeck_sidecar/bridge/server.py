"""HermesDeck gRPC server — Python Sidecar."""
from __future__ import annotations
import asyncio, json, logging
from concurrent import futures
from typing import Optional
import grpc
from hermesdeck_sidecar.bridge.client import BridgeClient
from hermesdeck_sidecar.bridge.tool_adapter import ToolAdapter
from hermesdeck_sidecar.hermes_adapter.agent_wrapper import HermesAgentWrapper
from hermesdeck_sidecar.hermes_adapter.memory_bridge import MemoryBridge
from hermesdeck_sidecar.hermes_adapter.skill_bridge import SkillBridge
from hermesdeck_sidecar.proto import hermesdeck_pb2 as pb
from hermesdeck_sidecar.proto import hermesdeck_pb2_grpc as pb_grpc

logger = logging.getLogger(__name__)


class SidecarServer:
    """Main server that bridges Go Runtime to Hermes Agent."""

    def __init__(self, port: int = 29552, go_addr: str = ""):
        self._port = port
        self._go_addr = go_addr
        self._server: Optional[grpc.aio.Server] = None
        self.tool_adapter = ToolAdapter()
        self.go_client = BridgeClient(target=go_addr)
        self.memory_bridge = MemoryBridge()
        self.skill_bridge = SkillBridge()
        self.agent: Optional[HermesAgentWrapper] = None

    async def start(self):
        self.agent = HermesAgentWrapper(
            tool_adapter=self.tool_adapter,
            memory_bridge=self.memory_bridge,
            skill_bridge=self.skill_bridge,
        )
        await self.agent.initialize()
        if self._go_addr:
            await self.go_client.connect(self._go_addr)
            self.tool_adapter.set_tool_executor(self.go_client.execute_tool)
        self._server = grpc.aio.server(
            futures.ThreadPoolExecutor(max_workers=10),
            options=[
                ("grpc.max_send_message_length", 100 * 1024 * 1024),
                ("grpc.max_receive_message_length", 100 * 1024 * 1024),
            ],
        )
        pb_grpc.add_HermesDeckBridgeServicer_to_server(
            _Servicer(
                agent=self.agent, tool_adapter=self.tool_adapter,
                go_client=self.go_client, memory_bridge=self.memory_bridge,
                skill_bridge=self.skill_bridge,
            ),
            self._server,
        )
        self._server.add_insecure_port(f"0.0.0.0:{self._port}")
        await self._server.start()
        logger.info("Python Sidecar listening on 0.0.0.0:%d", self._port)
        await self._server.wait_for_termination()

    async def stop(self):
        logger.info("Stopping Python Sidecar...")
        if self._server:
            await self._server.stop(5)
        await self.go_client.close()
        if self.agent:
            await self.agent.close()


class _Servicer(pb_grpc.HermesDeckBridgeServicer):
    def __init__(self, agent, tool_adapter, go_client, memory_bridge, skill_bridge):
        self._agent = agent
        self._tool_adapter = tool_adapter
        self._go_client = go_client
        self._memory_bridge = memory_bridge
        self._skill_bridge = skill_bridge

    async def HealthCheck(self, request, context):
        return pb.HealthResponse(
            service="python", status="alive",
            timestamp=int(asyncio.get_event_loop().time()), version="0.1.0",
        )

    async def RegisterTools(self, request, context):
        tools = [
            {"name": t.name, "description": t.description,
             "parameters_json_schema": t.parameters_json_schema}
            for t in request.tools
        ]
        count = self._tool_adapter.update_registry(tools)
        return pb.RegisterAck(success=True, registered_count=count)

    async def ProcessMessage(self, request, context):
        try:
            # Convert protobuf fields to plain Python types
            hist = list(request.history) if request.history else None
            async for resp in self._agent.process(
                session_id=request.session_id, message=request.user_message,
                history=hist,
            ):
                yield resp
        except Exception as e:
            logger.error("ProcessMessage error: %s | session_id=%s user_message=%s",
                         e, request.session_id, request.user_message)
            import traceback
            logger.error(traceback.format_exc())
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            yield pb.MessageResponse(
                session_id=request.session_id, is_final=True, text=f"[Error: {e}]"
            )

    async def ExecuteTool(self, request, context):
        args = json.loads(request.arguments_json) if request.arguments_json else {}
        result = await self._tool_adapter.execute_tool(
            request.tool_call_id, request.tool_name, args,
        )
        return pb.ToolResponse(
            tool_call_id=result.get("tool_call_id", ""),
            result_json=json.dumps(result.get("result", {})),
            is_error=result.get("is_error", False),
            error_message=result.get("error_message", ""),
        )
