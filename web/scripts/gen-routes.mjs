// Generates src/routeTree.gen.ts (git-ignored) without a full build: resolving the Vite config
// runs the TanStack Router plugin's generator. Used before typecheck / lint / test on a fresh clone.
import { resolveConfig } from 'vite'

await resolveConfig({ logLevel: 'silent' }, 'build')
