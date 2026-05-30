"""Hermes Agent wrapper — bridges Hermes AIAgent to gRPC message format with streaming and tool call routing.

Each session (project) gets its own AIAgent instance for complete context isolation.
"""
from __future__ import annotations
import asyncio, json, logging, os, sys, threading, time
from pathlib import Path
from typing import Any, AsyncGenerator, Dict, List, Optional

logger = logging.getLogger(__name__)

# ── Hermes Agent path setup ──────────────────────────────────────
_HERMES_AGENT_DIR = Path.home() / ".hermes" / "hermes-agent"
if str(_HERMES_AGENT_DIR) not in sys.path:
    sys.path.insert(0, str(_HERMES_AGENT_DIR))

_HERMES_VENV_SITE = (
    _HERMES_AGENT_DIR / "venv" / "lib" / f"python{sys.version_info.major}.{sys.version_info.minor}" / "site-packages"
)
if not _HERMES_VENV_SITE.exists():
    for p in (_HERMES_AGENT_DIR / "venv" / "lib").iterdir():
        sp = p / "site-packages"
        if sp.exists():
            _HERMES_VENV_SITE = sp
            break
if str(_HERMES_VENV_SITE) not in sys.path and _HERMES_VENV_SITE.exists():
    sys.path.insert(0, str(_HERMES_VENV_SITE))


# ── Agent-level identity ─────────────────────────────────────────
HERMESDECK_IDENTITY = (
    "# HermesDeck Chat Agent\n\n"
    "You are the HermesDeck web chat assistant — powered by Hermes "
    "Agent underneath. Answer questions naturally and directly. "
    "If someone asks what model you are, just say the name. "
    "Do not explain the architecture unless specifically asked.\n\n"
    "Chat-style behavior:\n"
    "- Answer simple questions immediately from your own knowledge.\n"
    "- Use tools for coding, file ops, research, debugging.\n"
    "- Respond naturally in the user's language.\n"
    "- Never say 'Ready. What do you need?' or report your internal state."
)

PLAN_MODE_SYSTEM_PROMPT = (
    "## PLAN MODE — READ-ONLY\n\n"
    "You are in PLAN MODE. Your job is to analyze, research, and create a plan — "
    "NOT to execute changes.\n\n"
    "STRICT RULES:\n"
    "- DO NOT use write_file, edit_file, or any filesystem write tool.\n"
    "- DO NOT execute bash commands that modify files (mkdir, rm, mv, touch, git commit, etc).\n"
    "- DO NOT use delegate_task or subagent tools for execution.\n"
    "- DO: read code, search files, analyze architecture, and write your plan to a markdown "
    "file under .pilotdeck/plans/ using write_file (plan directory writes are the ONLY exception).\n"
    "- DO: use exit_plan_mode when your plan is complete.\n\n"
    "If the user asks you to make changes, remind them to switch to Agent mode."
)

# Idle agent TTL — close agents not used for this many seconds
_AGENT_IDLE_TTL = 600  # 10 minutes
# Session history TTL — keep conversation memory across agent cleanups
_HISTORY_TTL = 86400  # 24 hours


def _build_agent_kwargs() -> Dict[str, Any]:
    """Build shared kwargs for AIAgent construction — called once at startup."""
    from run_agent import AIAgent  # noqa: trigger import side-effects

    kwargs: Dict[str, Any] = dict(
        quiet_mode=True,
        skip_context_files=True,
        enabled_toolsets=["*"],
        chat_type="web",
        platform="hermesdeck",
        ephemeral_system_prompt=HERMESDECK_IDENTITY,
    )

    # Load model from pilotdeck.yaml
    try:
        import yaml
        pilot_home = os.environ.get("PILOT_HOME", str(Path.home() / ".pilotdeck"))
        yaml_path = Path(pilot_home) / "pilotdeck.yaml"
        if yaml_path.exists():
            cfg = yaml.safe_load(yaml_path.read_text())
            agent_cfg = cfg.get("agent", {})
            model_full = agent_cfg.get("model", "") or ""
            if "/" in model_full:
                kwargs.setdefault("model", model_full.split("/", 1)[1])
            elif model_full:
                kwargs.setdefault("model", model_full)
            logger.info("Loaded model config from pilotdeck.yaml: %s", model_full)
    except Exception as exc:
        logger.info("pilotdeck.yaml config load skipped: %s", exc)

    return kwargs


class HermesAgentWrapper:
    """Manages per-session Hermes AIAgent instances with complete context isolation."""

    def __init__(
        self,
        tool_adapter: Any = None,
        memory_bridge: Any = None,
        skill_bridge: Any = None,
    ):
        self._tool_adapter = tool_adapter
        self._memory_bridge = memory_bridge
        self._skill_bridge = skill_bridge
        self._agent_kwargs: Optional[Dict[str, Any]] = None
        self._agents: Dict[str, Any] = {}  # session_id → AIAgent
        self._agent_last_used: Dict[str, float] = {}  # session_id → timestamp
        self._session_history: Dict[str, List[Dict[str, Any]]] = {}  # session_id → conversation messages
        self._session_locks: Dict[str, threading.Lock] = {}  # session_id → serialization lock
        self._cleanup_task: Optional[asyncio.Task] = None
        self._lock = threading.Lock()

    async def initialize(self):
        """Build shared kwargs once (model, prompt, etc.). Does NOT create agents yet."""
        if self._agent_kwargs is not None:
            return
        # Set HERMES_HOME so Hermes finds its config
        if "HERMES_HOME" not in os.environ:
            os.environ.setdefault(
                "HERMES_HOME",
                str(Path.home() / ".hermes" / "profiles" / "feishu-bot1"),
            )
        self._agent_kwargs = _build_agent_kwargs()
        # Start background cleanup loop
        self._cleanup_task = asyncio.ensure_future(self._cleanup_loop())
        logger.info("HermesAgentWrapper initialized (model=%s)", self._agent_kwargs.get("model", "default"))

    def _get_or_create_agent(self, session_id: str) -> Any:
        """Get existing agent for session, or create a new one."""
        from run_agent import AIAgent

        # Fast path with read lock
        agent = self._agents.get(session_id)
        if agent is not None:
            self._agent_last_used[session_id] = time.time()
            return agent

        # Slow path — create new agent
        with self._lock:
            # Double-check after acquiring lock
            agent = self._agents.get(session_id)
            if agent is not None:
                self._agent_last_used[session_id] = time.time()
                return agent

            agent = AIAgent(**self._agent_kwargs)
            self._agents[session_id] = agent
            self._agent_last_used[session_id] = time.time()
            logger.info("Created new AIAgent for session=%s (total=%d)", session_id, len(self._agents))
            return agent

    async def process(
        self, session_id: str, message: str, history: Optional[List[Dict[str, Any]]] = None,
        permission_mode: str = "default",
        project_path: str = "",
    ) -> AsyncGenerator[Any, None]:
        """Process a user message with streaming response for a specific session.

        Maintains conversation history per session_id so the agent remembers
        context even after idle cleanup recreates the AIAgent instance.
        """
        agent = self._get_or_create_agent(session_id)

        if agent is None:
            yield self._make_text_resp(session_id, "Agent not initialized", True, 0)
            return

        # ── Conversation history management ──
        # Initialize history for this session if not present
        if session_id not in self._session_history:
            self._session_history[session_id] = []

        # Use caller-provided history (from Gateway) as ground truth if given
        if history is not None and len(history) > len(self._session_history[session_id]):
            self._session_history[session_id] = list(history)

        # conversation_history passed to run_conversation is EXISTING history
        # (before this turn). run_conversation adds user_message internally.
        prev_history = list(self._session_history[session_id])

        # Resolve project workspace — prefer project_path from request metadata,
        # fall back to .cwd file / session key parsing.
        _workspace: Optional[str] = None
        _pilot_home = os.environ.get("PILOT_HOME", str(Path.home() / ".hermesdeck"))
        if project_path and os.path.isdir(project_path):
            _workspace = project_path
        else:
            _cwd_file = Path(_pilot_home) / "projects" / session_id / ".cwd"
            if _cwd_file.exists():
                _workspace = _cwd_file.read_text().strip()
            else:
                # Fallback: parse project path from session key.
                if "project=" in session_id:
                    _project_part = session_id.split("project=", 1)[1]
                    _project_path = _project_part.split(":s_", 1)[0] if ":s_" in _project_part else _project_part.split(":", 1)[0]
                    if _project_path and os.path.isdir(_project_path):
                        _workspace = _project_path

        # Build system message for plan mode
        _system_message: Optional[str] = None
        if permission_mode == "plan":
            _system_message = PLAN_MODE_SYSTEM_PROMPT
            logger.info("Plan mode system prompt injected for session=%s", session_id)

        # Per-session lock — serialize access to each AIAgent for thread safety
        if session_id not in self._session_locks:
            self._session_locks[session_id] = threading.Lock()
        session_lock = self._session_locks[session_id]

        from hermesdeck_sidecar.proto import hermesdeck_pb2 as pb

        queue: "threading.Queue" = __import__("queue").Queue(maxsize=256)
        _DONE = object()

        # ── Sync callbacks (called from agent thread) → thread-safe queue ──
        def _on_text(text: str):
            queue.put(("text", text))

        def _on_tool_start(tool_call_id: str, name: str, args_json: str):
            queue.put(("tool_start", tool_call_id, name, args_json))

        def _on_tool_end(tool_call_id: str, name: str, result: Any):
            queue.put(("tool_end", tool_call_id, name, result))

        def _run():
            _saved_cwd = os.getcwd()
            try:
                # Switch to project workspace so Hermes tools operate in the right directory
                if _workspace and os.path.isdir(_workspace):
                    os.chdir(_workspace)
                    os.environ["TERMINAL_CWD"] = _workspace
                # Per-session lock — only one thread runs on this agent at a time
                with session_lock:
                    # Set callbacks on this session's agent — no race because
                    # each session has its own agent instance.
                    agent.stream_delta_callback = _on_text
                    agent.tool_start_callback = _on_tool_start
                    agent.tool_complete_callback = _on_tool_end

                    agent.run_conversation(
                        user_message=message,
                        system_message=_system_message,
                        conversation_history=prev_history or None,
                    )
            except Exception as e:
                logger.error("Agent run error (session=%s): %s", session_id, e)
                queue.put(("error", str(e)))
            finally:
                os.chdir(_saved_cwd)
                queue.put(_DONE)

        thread = threading.Thread(target=_run, daemon=True)
        thread.start()

        # ── Async generator: poll thread-safe queue → yield gRPC responses ──
        seq = 0
        response_text_parts: List[str] = []
        while True:
            try:
                item = queue.get_nowait()
            except __import__("queue").Empty:
                if not thread.is_alive():
                    break
                await asyncio.sleep(0.05)
                continue
            if item is _DONE:
                break

            kind = item[0]

            if kind == "text":
                text = item[1]
                if text:
                    seq += 1
                    response_text_parts.append(text)
                    yield self._make_text_resp(session_id, text, False, seq)

            elif kind == "tool_start":
                _, call_id, name, args_json = item
                seq += 1
                try:
                    yield pb.MessageResponse(
                        session_id=session_id,
                        is_final=False,
                        sequence=seq,
                        tool_call=pb.ToolCallRequest(
                            tool_call_id=str(call_id or ""),
                            tool_name=str(name or ""),
                            arguments_json=str(args_json or "{}"),
                            session_id=str(session_id),
                        ),
                    )
                except Exception as tool_err:
                    logger.warning("ToolCallRequest creation failed: %s (call_id=%s name=%s)", tool_err, call_id, name)
                    # Still yield something so the turn doesn't hang
                    yield self._make_text_resp(session_id, f"[Tool: {name}]", False, seq)

            elif kind == "tool_end":
                pass

            elif kind == "error":
                seq += 1
                yield self._make_text_resp(session_id, f"[Error: {item[1]}]", True, seq)
                break

        # Final response marker
        if not thread.is_alive():
            seq += 1
            yield self._make_text_resp(session_id, "", True, seq)

        # Save user message + assistant response to session history
        # (user_message was NOT in prev_history — run_conversation added it internally)
        if session_id in self._session_history:
            self._session_history[session_id].append({"role": "user", "content": message})
        full_response = "".join(response_text_parts).strip()
        if full_response and session_id in self._session_history:
            self._session_history[session_id].append(
                {"role": "assistant", "content": full_response}
            )

    # ── Helpers ──────────────────────────────────────────────────

    def _make_text_resp(self, sid: str, text: str, final: bool, seq: int):
        from hermesdeck_sidecar.proto import hermesdeck_pb2 as pb
        return pb.MessageResponse(
            session_id=sid, is_final=final, sequence=seq, text=text,
        )

    async def _cleanup_loop(self):
        """Periodically close idle agent instances and stale session history."""
        while True:
            await asyncio.sleep(120)  # check every 2 minutes
            now = time.time()

            # Clean up idle agents (short TTL)
            stale_agents = [
                sid for sid, last in self._agent_last_used.items()
                if now - last > _AGENT_IDLE_TTL
            ]
            if stale_agents:
                with self._lock:
                    for sid in stale_agents:
                        agent = self._agents.pop(sid, None)
                        self._agent_last_used.pop(sid, None)
                        # Keep session history — survives agent cleanup
                        if agent is not None:
                            logger.info("Cleaned up idle agent session=%s", sid)

            # Clean up stale session history (long TTL)
            stale_history = [
                sid for sid in list(self._session_history.keys())
                if sid not in self._agent_last_used
                or now - self._agent_last_used.get(sid, 0) > _HISTORY_TTL
            ]
            if stale_history:
                for sid in stale_history:
                    self._session_history.pop(sid, None)
                    if stale_history:
                        logger.info("Cleaned up stale history session=%s (count=%d)", sid, len(stale_history))

    async def close(self):
        if self._cleanup_task:
            self._cleanup_task.cancel()
            try:
                await self._cleanup_task
            except asyncio.CancelledError:
                pass
        count = len(self._agents)
        self._agents.clear()
        self._agent_last_used.clear()
        logger.info("HermesAgentWrapper closed (cleaned %d session agents)", count)
