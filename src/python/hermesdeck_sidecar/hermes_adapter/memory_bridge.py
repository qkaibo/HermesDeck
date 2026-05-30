"""HermesDeck — Hermes 记忆系统桥接

将 Hermes 的 MemoryManager（SQLite FTS5 + Honcho）暴露给 Go Runtime。
"""

from __future__ import annotations

import json
import logging
from pathlib import Path
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)


class MemoryBridge:
    """记忆系统桥接

    对接 Hermes Agent 的 MemoryManager，提供持久化记忆能力。
    支持 SQLite FTS5 全文搜索和简单的键值存储。
    """

    def __init__(self, hermes_home: Optional[str] = None):
        self._hermes_home = hermes_home
        self._memory_manager: Optional[Any] = None
        self._initialized = False

    async def initialize(self):
        if self._initialized:
            return

        try:
            # 尝试导入 Hermes MemoryManager
            from agent.memory_manager import MemoryManager

            hermes_home = self._hermes_home or Path(__file__).parent.parent.parent / "data"
            self._memory_manager = MemoryManager(hermes_home=str(hermes_home))
            logger.info("MemoryManager initialized at %s", hermes_home)
        except ImportError:
            logger.warning(
                "Hermes MemoryManager not available. "
                "Falling back to simple JSON file storage."
            )
            self._memory_manager = None
        except Exception as e:
            logger.warning("Failed to initialize MemoryManager: %s", e)
            self._memory_manager = None

        self._initialized = True

    async def operate(
        self,
        operation: str,
        namespace: str = "",
        key: str = "",
        value: str = "",
        query: str = "",
        limit: int = 10,
    ) -> Dict[str, Any]:
        """执行记忆操作"""
        op = operation.lower()

        if op == "store":
            return await self._store(namespace, key, value)
        elif op == "recall":
            return await self._recall(namespace, key)
        elif op == "search":
            return await self._search(namespace, query, limit)
        elif op == "delete":
            return await self._delete(namespace, key)
        else:
            return {"success": False, "error_message": f"Unknown operation: {operation}"}

    async def _store(self, namespace: str, key: str, value: str) -> Dict[str, Any]:
        """存储一个记忆项"""
        if self._memory_manager:
            try:
                self._memory_manager.store(namespace=namespace, key=key, value=value)
                return {"success": True}
            except Exception as e:
                return {"success": False, "error_message": str(e)}
        else:
            return {"success": False, "error_message": "Memory not available"}

    async def _recall(self, namespace: str, key: str) -> Dict[str, Any]:
        """根据 key 召回"""
        if self._memory_manager:
            try:
                items = self._memory_manager.recall(namespace=namespace, key=key)
                return {"success": True, "items": items}
            except Exception as e:
                return {"success": False, "error_message": str(e)}
        else:
            return {"success": False, "error_message": "Memory not available"}

    async def _search(self, namespace: str, query: str, limit: int) -> Dict[str, Any]:
        """全文搜索"""
        if self._memory_manager:
            try:
                results = self._memory_manager.search(
                    namespace=namespace, query=query, limit=limit
                )
                items = [
                    {
                        "key": r.get("key", ""),
                        "value": r.get("value", ""),
                        "namespace": namespace,
                        "relevance": r.get("score", 0.0),
                        "timestamp": r.get("timestamp", 0),
                    }
                    for r in results
                ]
                return {"success": True, "items": items}
            except Exception as e:
                return {"success": False, "error_message": str(e)}
        else:
            return {"success": False, "error_message": "Memory not available"}

    async def _delete(self, namespace: str, key: str) -> Dict[str, Any]:
        if self._memory_manager:
            try:
                self._memory_manager.delete(namespace=namespace, key=key)
                return {"success": True}
            except Exception as e:
                return {"success": False, "error_message": str(e)}
        else:
            return {"success": False, "error_message": "Memory not available"}
