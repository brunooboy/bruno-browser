import { readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const headerLength = 12
const entryLength = 6
const aliasLength = 4
const brandedResourceIds = [101, 475]

function parsePack(payload) {
  if (payload.length < headerLength) throw new Error('Arquivo PAK truncado')
  const version = payload.readUInt32LE(0)
  if (version !== 5) throw new Error(`Formato PAK não suportado: ${version}`)

  const encoding = payload.readUInt8(4)
  if (encoding !== 1 && encoding !== 2) throw new Error(`Codificação PAK não suportada: ${encoding}`)

  const resourceCount = payload.readUInt16LE(8)
  const aliasCount = payload.readUInt16LE(10)
  const indexEnd = headerLength + ((resourceCount + 1) * entryLength)
  const aliasEnd = indexEnd + (aliasCount * aliasLength)
  if (aliasEnd > payload.length) throw new Error('Índice PAK truncado')

  const entries = []
  for (let index = 0; index <= resourceCount; index += 1) {
    const entryOffset = headerLength + (index * entryLength)
    entries.push({
      id: payload.readUInt16LE(entryOffset),
      offset: payload.readUInt32LE(entryOffset + 2),
    })
  }
  if (entries[0].offset !== aliasEnd || entries.at(-1).offset !== payload.length) {
    throw new Error('Offsets do PAK são inconsistentes')
  }

  const aliases = []
  for (let index = 0; index < aliasCount; index += 1) {
    const aliasOffset = indexEnd + (index * aliasLength)
    const entryIndex = payload.readUInt16LE(aliasOffset + 2)
    if (entryIndex >= resourceCount) throw new Error('Alias PAK aponta para uma entrada inválida')
    aliases.push({
      id: payload.readUInt16LE(aliasOffset),
      entryIndex,
    })
  }

  const resources = entries.slice(0, -1).map((entry, index) => {
    const end = entries[index + 1].offset
    if (entry.offset > end || end > payload.length) throw new Error('Recurso PAK possui limites inválidos')
    return Buffer.from(payload.subarray(entry.offset, end))
  })

  return {
    payload,
    version,
    encoding,
    resourceCount,
    aliasCount,
    indexEnd,
    aliasEnd,
    entries,
    aliases,
    resources,
  }
}

function resourceIndex(pack, resourceId) {
  const direct = pack.entries.slice(0, -1).findIndex((entry) => entry.id === resourceId)
  if (direct >= 0) return direct
  return pack.aliases.find((alias) => alias.id === resourceId)?.entryIndex ?? -1
}

function decode(pack, resource) {
  return resource.toString(pack.encoding === 2 ? 'utf16le' : 'utf8')
}

function encode(pack, text) {
  return Buffer.from(text, pack.encoding === 2 ? 'utf16le' : 'utf8')
}

function rebuild(pack) {
  const header = Buffer.from(pack.payload.subarray(0, headerLength))
  const index = Buffer.alloc((pack.resourceCount + 1) * entryLength)
  const aliases = Buffer.from(pack.payload.subarray(pack.indexEnd, pack.aliasEnd))
  let dataOffset = headerLength + index.length + aliases.length

  for (let entryIndex = 0; entryIndex < pack.resourceCount; entryIndex += 1) {
    const tableOffset = entryIndex * entryLength
    index.writeUInt16LE(pack.entries[entryIndex].id, tableOffset)
    index.writeUInt32LE(dataOffset, tableOffset + 2)
    dataOffset += pack.resources[entryIndex].length
  }
  const sentinelOffset = pack.resourceCount * entryLength
  index.writeUInt16LE(pack.entries.at(-1).id, sentinelOffset)
  index.writeUInt32LE(dataOffset, sentinelOffset + 2)
  return Buffer.concat([header, index, aliases, ...pack.resources], dataOffset)
}

function patchPack(pakPath) {
  const pack = parsePack(readFileSync(pakPath))
  let replacements = 0
  let brandedResources = 0
  const changedIndexes = new Set()
  const verifiedIndexes = new Set()

  for (const resourceId of brandedResourceIds) {
    const index = resourceIndex(pack, resourceId)
    if (index < 0) continue
    brandedResources += 1
    if (changedIndexes.has(index)) continue
    const original = decode(pack, pack.resources[index])
    if (original.includes('Bruno Engine') && !original.includes('Chromium')) {
      verifiedIndexes.add(index)
      continue
    }
    const branded = original.replaceAll('Chromium', 'Bruno Engine')
    if (branded === original) continue
    pack.resources[index] = encode(pack, branded)
    changedIndexes.add(index)
    verifiedIndexes.add(index)
    replacements += 1
  }

  writeFileSync(pakPath, rebuild(pack))
  const verified = parsePack(readFileSync(pakPath))
  for (const resourceId of brandedResourceIds) {
    const index = resourceIndex(verified, resourceId)
    if (index < 0 || !verifiedIndexes.has(index)) continue
    const text = decode(verified, verified.resources[index])
    if (!text.includes('Bruno Engine') || text.includes('Chromium')) {
      throw new Error(`${pakPath}: validação da identidade falhou no recurso ${resourceId}`)
    }
  }
  return { replacements, brandedResources }
}

function verifyRequiredBranding(pakPath) {
  const pack = parsePack(readFileSync(pakPath))
  for (const resourceId of brandedResourceIds) {
    const index = resourceIndex(pack, resourceId)
    if (index < 0) throw new Error(`${pakPath}: recurso obrigatório ${resourceId} ausente`)
    const text = decode(pack, pack.resources[index])
    if (!text.includes('Bruno Engine') || text.includes('Chromium')) {
      throw new Error(`${pakPath}: identidade ausente no recurso ${resourceId}`)
    }
  }
}

function inspect(pakPath) {
  const pack = parsePack(readFileSync(pakPath))
  console.log(`version=${pack.version} encoding=${pack.encoding} resources=${pack.resourceCount} aliases=${pack.aliasCount}`)
  for (let index = 0; index < pack.resourceCount; index += 1) {
    const text = decode(pack, pack.resources[index])
    if (/chromium|bruno engine/i.test(text)) console.log(`${pack.entries[index].id}\t${JSON.stringify(text)}`)
  }
}

const [command, target] = process.argv.slice(2)
if (!command || !target) {
  console.error('Uso: node scripts/brand-engine.mjs <inspect|patch-locales> <caminho>')
  process.exit(2)
}

if (command === 'inspect') {
  inspect(target)
} else if (command === 'patch-locales') {
  const packs = readdirSync(target).filter((name) => name.toLowerCase().endsWith('.pak')).sort()
  if (packs.length === 0) throw new Error(`Nenhum pacote de idioma encontrado em ${target}`)
  let replacements = 0
  let brandedResources = 0
  for (const name of packs) {
    const result = patchPack(join(target, name))
    replacements += result.replacements
    brandedResources += result.brandedResources
  }
  for (const required of ['en-US.pak', 'pt-BR.pak']) verifyRequiredBranding(join(target, required))
  console.log(`Identidade Bruno Engine validada em ${packs.length} pacotes (${brandedResources} recursos; ${replacements} alterados).`)
} else {
  throw new Error(`Comando desconhecido: ${command}`)
}
