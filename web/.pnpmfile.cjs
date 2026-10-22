// typescript-eslint (and ts-api-utils) still need the classic JS compiler API,
// which TypeScript 7 (native Go port) no longer ships. The project type-checks
// with TypeScript 7 (`tsc`), while the lint toolchain gets its own TypeScript 6.
const TS_FOR_LINT = '6.0.3'

function needsClassicTs(name) {
  return name === 'typescript-eslint' || name.startsWith('@typescript-eslint/') || name === 'ts-api-utils'
}

module.exports = {
  hooks: {
    readPackage(pkg) {
      if (needsClassicTs(pkg.name)) {
        if (pkg.peerDependencies) delete pkg.peerDependencies.typescript
        if (pkg.peerDependenciesMeta) delete pkg.peerDependenciesMeta.typescript
        pkg.dependencies = { ...pkg.dependencies, typescript: TS_FOR_LINT }
      }
      return pkg
    },
  },
}
