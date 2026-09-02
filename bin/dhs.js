#!/usr/bin/env node
// Shim: Node ≥24 rulează TypeScript nativ (type stripping), deci nu există pas de build.
await import('../src/cli.ts');
