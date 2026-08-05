// i18n 完整性审计：扫描 src 下所有 t()/$t() 使用的键，与语言包对比
import fs from 'fs'
import path from 'path'
import { pathToFileURL } from 'url'

const root = path.resolve('src')

const flatten = (obj, prefix = '', out = new Set()) => {
  for (const [k, v] of Object.entries(obj || {})) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) flatten(v, key, out)
    else out.add(key)
  }
  return out
}

const zh = (await import(pathToFileURL(path.resolve('src/i18n/locales/zh-CN.js')).href)).default
const en = (await import(pathToFileURL(path.resolve('src/i18n/locales/en-US.js')).href)).default
const zhKeys = flatten(zh)
const enKeys = flatten(en)

const files = []
const walk = (d) => {
  for (const e of fs.readdirSync(d, { withFileTypes: true })) {
    const p = path.join(d, e.name)
    if (e.isDirectory()) walk(p)
    else if (/\.(vue|js|ts)$/.test(e.name) && !p.includes(path.join('src', 'i18n'))) files.push(p)
  }
}
walk(root)

const used = new Map() // key -> Set(file)
const re = /\$?t\(\s*['"`]([a-zA-Z0-9_.]+)['"`]/g
for (const f of files) {
  const src = fs.readFileSync(f, 'utf8')
  let m
  while ((m = re.exec(src))) {
    const k = m[1]
    if (!k.includes('.')) continue
    if (!used.has(k)) used.set(k, new Set())
    used.get(k).add(path.relative(root, f))
  }
}

const missingZh = [...used.keys()].filter((k) => !zhKeys.has(k)).sort()
const missingEn = [...used.keys()].filter((k) => !enKeys.has(k)).sort()
const zhOnly = [...zhKeys].filter((k) => !enKeys.has(k)).sort()
const enOnly = [...enKeys].filter((k) => !zhKeys.has(k)).sort()

console.log(`扫描文件: ${files.length}，使用中的翻译键: ${used.size}`)
console.log(`zh-CN 键总数: ${zhKeys.size}，en-US 键总数: ${enKeys.size}`)
console.log(`\n===== 代码中使用但 zh-CN 缺失 (${missingZh.length}) =====`)
for (const k of missingZh) console.log(`${k}\t<- ${[...used.get(k)].join(', ')}`)
console.log(`\n===== 代码中使用但 en-US 缺失 (${missingEn.length}) =====`)
for (const k of missingEn) console.log(k)
console.log(`\n===== 仅 zh-CN 有、en-US 缺 (${zhOnly.length}) =====`)
console.log(zhOnly.slice(0, 80).join('\n'))
console.log(`\n===== 仅 en-US 有、zh-CN 缺 (${enOnly.length}) =====`)
console.log(enOnly.slice(0, 80).join('\n'))
