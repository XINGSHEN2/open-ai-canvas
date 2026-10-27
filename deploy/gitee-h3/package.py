"""Build the installable package from reviewed source, without credentials."""
import json
import zipfile
from pathlib import Path

root = Path(__file__).resolve().parent
manifest = json.loads((root / 'manifest.json').read_text())
documentation = (root / 'README.md').read_text()
manifest['documentation'] = documentation
with zipfile.ZipFile(root / 'gitee-h3.yingce-plugin', 'w', zipfile.ZIP_DEFLATED) as archive:
    archive.writestr('manifest.json', json.dumps(manifest, ensure_ascii=False, indent=2))
    archive.writestr('README.md', documentation)
    archive.writestr('docs/interface.md', documentation)
