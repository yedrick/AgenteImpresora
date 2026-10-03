#!/usr/bin/env bash
# Compila el servicio de ejemplo y los fragmentos de codigo de ANGULAR.md
# contra el paquete .tgz de verdad, en modo estricto.
#
# Hace falta porque ese codigo no lo compila nadie: esta en un .md y en una
# carpeta de ejemplos que no forma parte de ningun proyecto. Asi aparecieron
# tres errores que llevaban tiempo ahi, uno de ellos en el bloque que la
# propia guia te pide copiar.
set -euo pipefail

RAIZ="$(cd "$(dirname "$0")/.." && pwd)"
TRABAJO="${TMPDIR:-/tmp}/collatech-ejemplos"

cd "$RAIZ/collatech-sdk"
[ -d node_modules ] || npm ci --silent
npm run build --silent >/dev/null
rm -f ./*.tgz && npm pack --silent >/dev/null
PAQUETE="$(ls "$RAIZ"/collatech-sdk/*.tgz | head -1)"

rm -rf "$TRABAJO"; mkdir -p "$TRABAJO/src"; cd "$TRABAJO"
npm init -y >/dev/null
npm install --silent "$PAQUETE" >/dev/null

cp "$RAIZ"/examples/angular/*.ts src/
cp "$RAIZ"/examples/angular/*.d.ts src/ 2>/dev/null || true
# La guia importa el servicio desde ./services/, que es donde te pide
# copiarlo. Se reexporta ahi para que esa ruta tambien resuelva.
mkdir -p src/services
echo "export * from '../collatech-print.service';" > src/services/collatech-print.service.ts

# Los bloques ```typescript de la guia, envueltos para que compilen sueltos.
python3 - "$RAIZ/ANGULAR.md" "$TRABAJO/src" <<'PY'
import re, sys, pathlib
guia, destino = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
bloques = re.findall(r'```typescript\n(.*?)```', guia.read_text(encoding='utf-8'), re.S)

# Un bloque que importa un archivo del proyecto del lector (./app.component,
# ../services/printer.service) no se puede compilar aqui: ese archivo lo crea
# quien sigue la guia. Se apartan y se dice cuantos son, para no aparentar
# que se comprueba mas de lo que se comprueba.
def importa_del_proyecto(b):
    for m in re.findall(r"from\s+'([^']+)'", b):
        if m.startswith('.') and 'collatech-print.service' not in m:
            return True
    return False

apartados = [b for b in bloques if importa_del_proyecto(b)]
revisables = [b for b in bloques if not importa_del_proyecto(b)]
sueltos, clases = [], []
for b in revisables:
    (clases if re.search(r'^\s*(import|@Component|@Injectable|export)', b, re.M) else sueltos).append(b)
for i, b in enumerate(clases):
    (destino / f'guia-clase-{i}.ts').write_text(b, encoding='utf-8')
print(f"  bloques de ANGULAR.md: {len(clases)} completos + {len(sueltos)} fragmentos")
if apartados:
    print(f"  apartados {len(apartados)}: importan archivos que crea el lector")
PY

cat > tsconfig.json <<'EOF'
{
  "compilerOptions": {
    "target": "es2020", "module": "esnext", "moduleResolution": "bundler",
    "strict": true, "experimentalDecorators": true, "skipLibCheck": true,
    "lib": ["es2020", "dom"], "noEmit": true
  },
  "include": ["src/**/*.ts"]
}
EOF

echo "Comprobando tipos..."
"$RAIZ/collatech-sdk/node_modules/.bin/tsc" -p tsconfig.json
echo "El servicio de ejemplo y el codigo de la guia compilan."
