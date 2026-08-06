import { existsSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { spawnSync } from 'node:child_process'

const argumentsToNpm = process.argv.slice(2)
const bundledNpm = join(dirname(process.execPath), 'node_modules', 'npm', 'bin', 'npm-cli.js')
const npmEnvironment = {
  ...process.env,
  // Wails builds in production mode, but TypeScript, Vite and Tailwind are
  // build-time dependencies and must still be installed by `npm ci`.
  npm_config_omit: '',
  npm_config_include: 'dev',
  npm_config_production: 'false',
}

const result = existsSync(bundledNpm)
  ? spawnSync(process.execPath, [bundledNpm, ...argumentsToNpm], { env: npmEnvironment, stdio: 'inherit' })
  : spawnSync(process.platform === 'win32' ? 'npm.cmd' : 'npm', argumentsToNpm, { env: npmEnvironment, stdio: 'inherit' })

if (result.error) {
  console.error(result.error.message)
  process.exit(1)
}

const exitCode = result.status ?? 1
const nodeModulesDirectory = join(process.cwd(), 'node_modules')
if (exitCode === 0 && existsSync(nodeModulesDirectory)) {
  // Keep Go's recursive package discovery out of JavaScript dependencies.
  writeFileSync(join(nodeModulesDirectory, 'go.mod'), 'module bruno-browser/frontend-dependencies\n\ngo 1.26\n')
}

process.exit(exitCode)
