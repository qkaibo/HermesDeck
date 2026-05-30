"""
HermesDeck HTTP Bridge — OpenAI /v1/chat/completions compatible endpoint.

PilotDeck's TurnRunner calls this instead of DeepSeek API directly.
The Hermes AIAgent handles the request (runs tools internally, generates text).
Streams responses back in SSE format.
"""
from __future__ import annotations
import asyncio, json, logging, os, uuid
from pathlib import Path
from typing import Any, AsyncGenerator, Dict, List, Optional

import aiohttp
from aiohttp import web

from hermesdeck_sidecar.hermes_adapter.agent_wrapper import HermesAgentWrapper

logger = logging.getLogger(__name__)

# ── Globals ──
_wrapper: Optional[HermesAgentWrapper] = None
_agent_lock = asyncio.Lock()


async def get_wrapper() -> HermesAgentWrapper:
    global _wrapper
    if _wrapper is not None:
        return _wrapper
    async with _agent_lock:
        if _wrapper is not None:
            return _wrapper
        w = HermesAgentWrapper()
        await w.initialize()
        _wrapper = w
        logger.info("HermesAgentWrapper initialized for HTTP bridge")
        return _wrapper


# ── Request parsing ──

def extract_user_message(messages: List[Dict[str, Any]]) -> tuple[str, List[Dict[str, Any]]]:
    """Extract the last user message and return (user_message, history).

    History includes all non-system, non-tool messages before the last user message.
    System messages and tool messages are excluded — the AIAgent has its own system prompt
    and handles tool calls internally.
    """
    # Find last user message
    last_user_idx = -1
    for i in range(len(messages) - 1, -1, -1):
        if messages[i].get("role") == "user":
            last_user_idx = i
            break

    if last_user_idx < 0:
        return "", []

    user_message = messages[last_user_idx].get("content", "")
    if isinstance(user_message, list):
        # Extract text from content array
        texts = []
        for block in user_message:
            if isinstance(block, dict) and block.get("type") == "text":
                texts.append(block.get("text", ""))
        user_message = "\n".join(texts)

    # History = everything before last user message, excluding system and tool roles
    history = []
    for msg in messages[:last_user_idx]:
        role = msg.get("role", "")
        if role in ("system", "tool"):
            continue
        content = msg.get("content", "")
        if isinstance(content, list):
            texts = []
            for block in content:
                if isinstance(block, dict) and block.get("type") == "text":
                    texts.append(block.get("text", ""))
            content = "\n".join(texts)
        if content:
            history.append({"role": role, "content": content})

    return user_message, history


# ── SSE streaming ──

def make_sse_chunk(content: str, index: int = 0, finish_reason: Optional[str] = None) -> str:
    """Build an SSE data line matching OpenAI chat.completions.chunk format."""
    chunk_id = f"chatcmpl-{uuid.uuid4().hex[:8]}"
    choice = {
        "index": index,
        "delta": {"content": content} if content else {},
        "finish_reason": finish_reason,
    }
    data = {
        "id": chunk_id,
        "object": "chat.completion.chunk",
        "choices": [choice],
    }
    return f"data: {json.dumps(data, ensure_ascii=False)}\n\n"


# ── HTTP handlers ──

async def handle_chat_completions(request: web.Request) -> web.StreamResponse:
    """POST /v1/chat/completions — OpenAI-compatible chat endpoint."""
    try:
        body = await request.json()
    except json.JSONDecodeError:
        raise web.HTTPBadRequest(text=json.dumps({"error": "invalid json"}), content_type="application/json")

    # Build a unique session ID for each HTTP request so concurrent
    # requests don't contend on the same session_lock in agent_wrapper.
    req_session_id = request.headers.get("X-Session-Id") or f"web-{uuid.uuid4().hex[:8]}"
    messages = body.get("messages", [])
    stream = body.get("stream", False)

    # Read permission mode from metadata (set by HermesDeck Gateway for plan mode)
    _metadata = body.get("metadata", {}) or {}
    permission_mode = _metadata.get("permissionMode", "default")

    # Read project path from metadata so sidecar knows the workspace
    project_path = _metadata.get("projectPath", "")

    if not messages:
        raise web.HTTPBadRequest(text=json.dumps({"error": "no messages"}), content_type="application/json")

    user_message, history = extract_user_message(messages)
    if not user_message:
        raise web.HTTPBadRequest(text=json.dumps({"error": "no user message"}), content_type="application/json")

    wrapper = await get_wrapper()

    if stream:
        # Streaming response
        resp = web.StreamResponse(
            status=200,
            reason="OK",
            headers={
                "Content-Type": "text/event-stream",
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
                "X-Accel-Buffering": "no",
            },
        )
        await resp.prepare(request)

        # Send initial role chunk (OpenAI format requires this for streaming)
        await resp.write(make_sse_chunk("").encode("utf-8"))

        # Process through Hermes Agent
        try:
            async for msg in wrapper.process(
                session_id=req_session_id,
                message=user_message,
                history=history if history else None,
                permission_mode=permission_mode,
                project_path=project_path,
            ):
                text = msg.text if hasattr(msg, "text") else ""
                if text:
                    await resp.write(make_sse_chunk(text).encode("utf-8"))
        except Exception as e:
            logger.error("Agent process error: %s", e)
            await resp.write(make_sse_chunk("", finish_reason="error").encode("utf-8"))

        # Send done signal
        await resp.write(make_sse_chunk("", finish_reason="stop").encode("utf-8"))
        await resp.write(b"data: [DONE]\n\n")
        return resp

    else:
        # Non-streaming response (fallback)
        collected = []
        async for msg in wrapper.process(
            session_id=req_session_id,
            message=user_message,
            history=history if history else None,
            permission_mode=permission_mode,
            project_path=project_path,
        ):
            text = msg.text if hasattr(msg, "text") else ""
            if text:
                collected.append(text)

        result = {
            "id": f"chatcmpl-{uuid.uuid4().hex[:8]}",
            "object": "chat.completion",
            "choices": [{
                "index": 0,
                "message": {"role": "assistant", "content": "".join(collected)},
                "finish_reason": "stop",
            }],
            "usage": {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
        }
        return web.json_response(result)


async def handle_health(request: web.Request) -> web.Response:
    """GET /health"""
    return web.json_response({"status": "ok", "service": "hermesdeck-http-bridge"})


# ── Server startup ──

def create_app() -> web.Application:
    app = web.Application()
    app.router.add_post("/v1/chat/completions", handle_chat_completions)
    app.router.add_get("/health", handle_health)
    return app


def main():
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    )
    port = int(os.environ.get("HERMESDECK_HTTP_PORT", "29553"))
    app = create_app()
    web.run_app(app, host="0.0.0.0", port=port, print=lambda *a: logger.info(" ".join(str(x) for x in a)))


if __name__ == "__main__":
    main()
