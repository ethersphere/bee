import {
  
    WASI,
  } from 'node:wasi'

import fs from 'node:fs/promises'
import path from 'node:path'
import { env } from 'node:process';

const wasi = new WASI({
  version: 'preview1',
  env,
  args: ['printconfig']
})
const bytes = await fs.readFile(
  path.join(import.meta.dirname, './bee'),
)
const { instance } = await WebAssembly.instantiate(bytes, {
  wasi_snapshot_preview1: wasi.wasiImport,
})

wasi.start(instance)