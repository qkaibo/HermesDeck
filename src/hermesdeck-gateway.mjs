#!/usr/bin/env node
/**
 * HermesDeck Gateway — clean PilotDeck instance.
 *
 * No monkey-patches. PilotDeck's native submitTurn calls the LLM provider
 * configured in pilotdeck.yaml. The 'hermesdeck' provider points to our
 * Python HTTP bridge (port 29553) which runs the Hermes AIAgent.
 *
 * PilotDeck handles:
 *   - SessionRouter (per-session AgentSession, beginTurn/endTurn)
 *   - Transcript (JSONL on disk via TurnRunner)
 *   - Message history (state.messages, automatic)
 *   - 30-minute idle eviction
 */
import { createLocalGateway } from '../dist/src/cli/createLocalGateway.js';
import { startPilotDeckServer } from '../dist/src/cli/pilotdeckServer.js';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PORT = parseInt(process.env.HERMESDECK_GATEWAY_PORT || '28788', 10);
const PILOT_HOME = process.env.PILOT_HOME || path.resolve(process.env.HOME || '/home/ts', '.hermesdeck');

process.env.PILOT_HOME = PILOT_HOME;

const { gateway, configStore, dispose } = createLocalGateway();

const server = await startPilotDeckServer({
  gateway,
  port: PORT,
  host: '127.0.0.1',
});

console.log(`[hermesdeck-gateway] listening on :${PORT}`);
console.log(`[hermesdeck-gateway] PILOT_HOME=${PILOT_HOME}`);

process.on('SIGTERM', () => {
  dispose();
  server.close(() => process.exit(0));
});
