// Quita binarios nativos de otras plataformas y las librerías de GPU
// (CUDA/TensorRT), que no se usan y ocupan cientos de MB.
// Uso: node prune.cjs <carpeta node_modules> <linux|win32|android> <x64|arm64>
const fs = require('fs'), path = require('path');
const [root, os, arch] = process.argv.slice(2);
let freed = 0;
const size = p => { let s = 0; try { const st = fs.lstatSync(p); if (st.isDirectory()) for (const f of fs.readdirSync(p)) s += size(path.join(p, f)); else s = st.size; } catch {} return s; };
const rm = p => { freed += size(p); fs.rmSync(p, { recursive: true, force: true }); };
const ortOS = os === 'android' ? 'linux' : os;
const ort = path.join(root, 'onnxruntime-node', 'bin');
if (fs.existsSync(ort)) {
  for (const napi of fs.readdirSync(ort)) {
    const d = path.join(ort, napi);
    for (const o of fs.readdirSync(d)) {
      if (o !== ortOS) { rm(path.join(d, o)); continue; }
      for (const a of fs.readdirSync(path.join(d, o))) {
        const ad = path.join(d, o, a);
        if (a !== arch) { rm(ad); continue; }
        for (const f of fs.readdirSync(ad)) if (/cuda|tensorrt|DirectML/i.test(f)) rm(path.join(ad, f));
      }
    }
  }
}
// Copias anidadas de onnxruntime-node dentro de otros paquetes.
(function walk(dir, depth) {
  if (depth > 4) return;
  let ents; try { ents = fs.readdirSync(dir, { withFileTypes: true }); } catch { return; }
  for (const e of ents) {
    if (!e.isDirectory()) continue;
    const p = path.join(dir, e.name);
    if (e.name === 'onnxruntime-node' && p !== path.join(root, 'onnxruntime-node')) {
      require('child_process').execFileSync(process.execPath, [__filename, path.dirname(p), os, arch], { stdio: 'inherit' });
      continue;
    }
    walk(p, depth + 1);
  }
})(root, 0);
console.log(`prune ${root}: ${(freed / 1e6).toFixed(0)} MB liberados`);
