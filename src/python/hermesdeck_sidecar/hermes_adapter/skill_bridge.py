"""Skill system bridge — manages SKILL.md skills."""
from __future__ import annotations
import logging
from pathlib import Path
from typing import Any, Dict, Optional

logger = logging.getLogger(__name__)


class SkillBridge:
    """Bridges Hermes skill system to Go Runtime."""

    def __init__(self, hermes_home: Optional[str] = None):
        self._hermes_home = hermes_home
        self._skills_dir: Optional[Path] = None
        self._initialized = False

    async def initialize(self):
        if self._initialized:
            return
        h = Path(self._hermes_home or Path(__file__).parent.parent.parent / "data")
        self._skills_dir = h / "skills"
        self._skills_dir.mkdir(parents=True, exist_ok=True)
        self._initialized = True

    async def operate(self, operation: str, skill_name: str = "", skill_content: str = "") -> Dict[str, Any]:
        op = operation.lower()
        if op == "list":
            return await self._list()
        elif op == "install":
            return await self._install(skill_name, skill_content)
        elif op == "remove":
            return await self._remove(skill_name)
        elif op == "info":
            return await self._info(skill_name)
        return {"success": False, "error_message": f"Unknown operation: {operation}"}

    async def _list(self) -> Dict[str, Any]:
        if not self._skills_dir or not self._skills_dir.exists():
            return {"success": True, "skills": []}
        skills = []
        for sf in self._skills_dir.glob("**/SKILL.md"):
            skills.append({
                "name": sf.parent.name,
                "description": sf.read_text(encoding="utf-8")[:100],
                "version": "1.0", "is_active": True,
                "installed_at": int(sf.stat().st_mtime),
            })
        return {"success": True, "skills": skills}

    async def _install(self, name: str, content: str) -> Dict[str, Any]:
        if not name or not content:
            return {"success": False, "error_message": "name and content required"}
        d = self._skills_dir / name
        d.mkdir(parents=True, exist_ok=True)
        (d / "SKILL.md").write_text(content, encoding="utf-8")
        return {"success": True}

    async def _remove(self, name: str) -> Dict[str, Any]:
        import shutil
        d = self._skills_dir / name
        if not d.exists():
            return {"success": False, "error_message": f"Skill not found: {name}"}
        shutil.rmtree(d)
        return {"success": True}

    async def _info(self, name: str) -> Dict[str, Any]:
        sf = self._skills_dir / name / "SKILL.md"
        if not sf.exists():
            return {"success": False, "error_message": f"Skill not found: {name}"}
        return {
            "success": True,
            "skills": [{
                "name": name, "description": sf.read_text(encoding="utf-8")[:200],
                "version": "1.0", "is_active": True,
                "installed_at": int(sf.stat().st_mtime),
            }],
        }
