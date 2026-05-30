"""HermesDeck — gRPC 桥接: 工具适配器"""

from __future__ import annotations

import json
import logging
from typing import Any, Callable, Dict, List, Optional

logger = logging.getLogger(__name__)


class ToolAdapter:
    """HermesDeck 工具适配器

    管理 Go Runtime 注册的工具列表，并提供工具调用接口。
    """

    def __init__(self):
        self._tools: Dict[str, Dict[str, Any]] = {}
        self._tool_executor: Optional[Callable] = None

    def set_tool_executor(self, executor: Callable):
        """设置工具执行器 — 负责将工具调用发往 Go Runtime"""
        self._tool_executor = executor

    def update_registry(self, tools: List[Dict[str, Any]]) -> int:
        """更新 Go 端注册的工具列表"""
        for tool in tools:
            name = tool.get("name", "")
            self._tools[name] = tool
        logger.info("Tool registry updated: %d tools", len(self._tools))
        return len(self._tools)

    def get_tools_for_llm(self) -> List[Dict[str, Any]]:
        """返回工具列表（格式化为 LLM 可识别的 function calling 格式）"""
        tools = []
        for name, defn in self._tools.items():
            params_raw = defn.get("parameters_json_schema", "{}")
            if isinstance(params_raw, str):
                try:
                    params = json.loads(params_raw)
                except json.JSONDecodeError:
                    params = {"type": "object", "properties": {}}
            else:
                params = params_raw

            tools.append({
                "type": "function",
                "function": {
                    "name": name,
                    "description": defn.get("description", ""),
                    "parameters": params,
                },
            })
        return tools

    async def execute_tool(self, tool_call_id: str, tool_name: str, arguments: Dict[str, Any]) -> Dict[str, Any]:
        """通过 Go Runtime 执行工具"""
        if self._tool_executor is None:
            return {
                "tool_call_id": tool_call_id,
                "is_error": True,
                "error_message": "Tool executor not set — no Go Runtime connection.",
            }

        # 组装请求并发送到 Go
        result = await self._tool_executor(tool_call_id, tool_name, arguments)

        if result.get("is_error"):
            logger.warning("Tool %s failed: %s", tool_name, result.get("error_message", ""))

        return result

    def list_tools(self) -> List[Dict[str, Any]]:
        return list(self._tools.values())

    def get_tool(self, name: str) -> Optional[Dict[str, Any]]:
        return self._tools.get(name)
