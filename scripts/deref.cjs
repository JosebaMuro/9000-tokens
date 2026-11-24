// Sustituye los enlaces de los workspaces de npm por copias reales,
// para que el paquete funcione igual en Windows, Linux y Android.
const fs = require('fs'), path = require('path');
const root = process.argv[2];
function walk(dir, depth) {
  if (depth > 2) return;
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    let st; try { st = fs.lstatSync(p); } catch { continue; }
    if (st.isSymbolicLink()) {
      const real = fs.realpathSync(p);
      if (fs.statSync(real).isDirectory()) {
        fs.rmSync(p, { recursive: true, force: true });
        fs.cpSync(real, p, { recursive: true, dereference: true });
        console.log('copiado', path.relative(root, p));
      }
    } else if (st.isDirectory() && e.name.startsWith('@')) walk(p, depth + 1);
  }
}
walk(path.join(root, 'node_modules'), 0);
