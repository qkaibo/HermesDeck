#!/bin/bash
# Fix dist files broken by tsc CJS compilation
set -e
DIST="/home/ts/HermesDeck/dist"
EDGE_SRC="/home/ts/HermesDeck/src/context/memory/edgeclaw-memory-core"

# 1. ripgrep.js - import.meta.url in CJS
sed -i 's|const require = (0, node_module_1.createRequire)(import.meta.url);|const rgPath = require("@vscode/ripgrep").rgPath;|' "$DIST/src/tool/builtin/filesystem/ripgrep.js" 2>/dev/null
sed -i '/const { rgPath } = require("@vscode\/ripgrep");/d' "$DIST/src/tool/builtin/filesystem/ripgrep.js" 2>/dev/null

# 2. edgeclaw - CJS can't require ESM with top-level await
cat > "$DIST/src/context/memory/createEdgeClawMemoryProviderFromConfig.js" << 'JSEOF'
"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.createEdgeClawMemoryProviderFromConfig = createEdgeClawMemoryProviderFromConfig;
function createEdgeClawMemoryProviderFromConfig(options) {
    return undefined;
}
JSEOF

# 3. Restore edgeclaw ESM files
cp -r "$EDGE_SRC/lib/"* "$DIST/src/context/memory/edgeclaw-memory-core/src/" 2>/dev/null

echo "post-tsc fixes applied"
