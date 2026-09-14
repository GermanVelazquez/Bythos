// Genera los PNG del icono Bythos con solo Node stdlib (zlib + fs).
// Dibuja: cuadrado negro redondeado + B blanca geometrica + ondas en contadores.
// Uso desde esta carpeta: node gen-icons.mjs
import { writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { deflateSync } from 'node:zlib';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');

const BG = [11, 11, 12, 255];    // #0B0B0C
const FG = [255, 255, 255, 255]; // B blanca

// Tabla CRC32 para los chunks PNG.
const CRC_T = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();

function crc32(buf) {
  let c = 0xffffffff;
  for (let i = 0; i < buf.length; i++) c = CRC_T[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

function chunk(tipo, datos) {
  const len = Buffer.alloc(4);
  len.writeUInt32BE(datos.length);
  const td = Buffer.concat([Buffer.from(tipo, 'ascii'), datos]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(td));
  return Buffer.concat([len, td, crc]);
}

// B geometrica en canonico 512, escalada por s.
function dibujar(N) {
  const s = N / 512;
  const px = Buffer.alloc(N * N * 4, 0);
  const set = (x, y, c) => {
    if (x < 0 || y < 0 || x >= N || y >= N) return;
    const o = (y * N + x) * 4;
    px[o] = c[0]; px[o + 1] = c[1]; px[o + 2] = c[2]; px[o + 3] = 255;
  };
  const rect = (x0, y0, x1, y1, c) => {
    for (let y = Math.round(y0 * s); y < Math.round(y1 * s); y++)
      for (let x = Math.round(x0 * s); x < Math.round(x1 * s); x++) set(x, y, c);
  };

  // Fondo redondeado.
  const R = Math.round(112 * s);
  for (let y = 0; y < N; y++) {
    for (let x = 0; x < N; x++) {
      const cx = Math.min(x, N - 1 - x);
      const cy = Math.min(y, N - 1 - y);
      const dx = R - 1 - cx, dy = R - 1 - cy;
      const dentro = dx <= 0 || dy <= 0 || dx * dx + dy * dy <= R * R;
      if (dentro) set(x, y, BG);
    }
  }

  // B: asta + 3 barras + postes derechos (contadores quedan en negro).
  rect(148, 108, 196, 404, FG);  // asta
  rect(148, 108, 368, 156, FG);  // barra alta
  rect(148, 232, 368, 280, FG);  // barra media
  rect(148, 356, 368, 404, FG);  // barra baja
  rect(320, 108, 368, 280, FG);  // poste alto
  rect(320, 232, 368, 404, FG);  // poste bajo

  // Ondas blancas dentro de cada contador.
  const onda = (midY) => {
    const th = Math.max(1, Math.round(7 * s));
    for (let x = Math.round(200 * s); x < Math.round(316 * s); x++) {
      const wy = midY * s + Math.sin(((x / s) - 200) * (2 * Math.PI / 62)) * 14 * s;
      for (let d = -th; d <= th; d++) set(x, Math.round(wy) + d, FG);
    }
  };
  onda(194);
  onda(318);

  // Empaqueta PNG con filtro 0 por fila.
  const raw = Buffer.alloc(N * (N * 4 + 1));
  for (let y = 0; y < N; y++) {
    raw[y * (N * 4 + 1)] = 0;
    px.copy(raw, y * (N * 4 + 1) + 1, y * N * 4, (y + 1) * N * 4);
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(N, 0); ihdr.writeUInt32BE(N, 4);
  ihdr[8] = 8; ihdr[9] = 6; // 8 bits, RGBA
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk('IHDR', ihdr),
    chunk('IDAT', deflateSync(raw, { level: 9 })),
    chunk('IEND', Buffer.alloc(0)),
  ]);
}

const salidas = [
  ['assets/icon-512.png', 512],
  ['assets/icon-256.png', 256],
  ['extension/icons/icon128.png', 128],
  ['extension/icons/icon48.png', 48],
  ['extension/icons/icon16.png', 16],
  ['desktop/ui/public/favicon.png', 64],
];
for (const [rel, n] of salidas) {
  writeFileSync(join(ROOT, rel), dibujar(n));
  console.log(`ok ${rel} (${n}x${n})`);
}
