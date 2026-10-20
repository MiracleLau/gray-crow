import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

const root = new URL('../src', import.meta.url).pathname
const keys = (file) => new Set([...readFileSync(file, 'utf8').matchAll(/^  '([^']+)':/gm)].map((m) => m[1]))
const zh = keys(join(root, 'i18n/zh.ts'))
const en = keys(join(root, 'i18n/en.ts'))

const walk = (dir) =>
  readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    return statSync(path).isDirectory() ? walk(path) : path.endsWith('.vue') || path.endsWith('.ts') ? [path] : []
  })

const problems = []
for (const file of walk(root)) {
  if (file.includes('/i18n/')) continue
  const src = readFileSync(file, 'utf8')
  for (const [, key] of src.matchAll(/\bt\(\s*'([^']+)'/g)) {
    if (!zh.has(key)) problems.push(`${file.replace(root, 'src')}: 中文缺失 ${key}`)
    if (!en.has(key)) problems.push(`${file.replace(root, 'src')}: 英文缺失 ${key}`)
  }
}
for (const key of zh) if (!en.has(key)) problems.push(`en.ts 缺少 ${key}`)
for (const key of en) if (!zh.has(key)) problems.push(`zh.ts 缺少 ${key}`)

if (problems.length) {
  console.error(problems.join('\n'))
  process.exit(1)
}
console.log(`i18n ok: ${zh.size} 条文案，中英对齐`)
