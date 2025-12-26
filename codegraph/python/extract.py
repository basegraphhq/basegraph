import time
import os
import ast
from pathlib import Path
import NodeCollector
import json
import shutil
from git_support import GitSupport
from ast_utils import extract_docstring

def extract_ast_nodes(filepath, qualified_file_name, package_root: str | None = None):
    with open(filepath, "r", encoding="utf-8") as source:
        code = source.read()

    try:
        tree = ast.parse(code)
    except SyntaxError:
        return

    # Split source into lines for code snippet extraction
    source_lines = code.splitlines()

    # use the per-file qualified name as the module namespace
    collector = NodeCollector.NodeCollector(
        module_name=qualified_file_name,
        source_file=qualified_file_name,
        source_lines=source_lines,
        filepath=filepath,
    )
    collector.visit(tree)

    # Add module node with docstring (keyed by the file-qualified name)
    docstring = extract_docstring(tree)
    collector.result[qualified_file_name] = {
        "kind": "module",
        "name": qualified_file_name,
        "pos": {"start": 1, "end": len(source_lines)},
        "ast_type": "Module",
        "docstring": docstring,
        "relations": [],
        "qualified_name": qualified_file_name,
        "parent_qualified_name": None,
        "code": None,  # Module-level code not stored (too large)
        "filepath": filepath,
        "language": "python",
    }

    return collector.result
    

def extract(pkgstr: str, dir:str):
    seen_namespaces = set()
    result = {}

    start = time.time()
    for root, _, files in os.walk(dir):
        for filename in files:
            if not filename.endswith(".py"):
                continue
            path = os.path.join(root, filename )
            qualified_file_name = _derive_qualified_name(dir, path, pkgstr)
            if qualified_file_name not in seen_namespaces:
                seen_namespaces.add(qualified_file_name)
                nodes_dict = extract_ast_nodes(path, qualified_file_name)
                if nodes_dict and len(nodes_dict):
                    result.update(nodes_dict)

    elapsed = time.time() - start
    return result, elapsed


def _derive_qualified_name(root:str, path:str, pkgstr:str) -> str:
    rel_path = os.path.relpath(path, root)
    if rel_path.endswith(".py"):
        rel_path = rel_path[: -len(".py")]
    rel_path = rel_path.replace(os.sep, ".")
    if rel_path.endswith(".__init__"):
        rel_path = rel_path[: -len(".__init__")]
    rel_path = rel_path.strip(".")
    if pkgstr:
        if rel_path:
            return f"{pkgstr}.{rel_path}"
        return pkgstr
    return rel_path

def extract_cli() -> None:
    import argparse

    parser = argparse.ArgumentParser(description="Extract Python codegraph")
    parser.add_argument("--pkg", dest="pkg", default="", help="Package/module prefix (e.g. github.com/org/repo)")
    parser.add_argument("--dir", dest="dir", default=".", help="Root directory to walk (ignored if --repo set)")
    parser.add_argument("--repo", dest="repo", default="", help="Git repo URL to clone and index")
    parser.add_argument("--ref", dest="ref", default="", help="Git ref (branch/tag/sha) to checkout when cloning")
    parser.add_argument(
        "--keep-clone",
        dest="keep_clone",
        action="store_true",
        help="Keep the cloned repository directory (default: delete)",
    )
    parser.add_argument(
        "--out",
        dest="out",
        default="output/output.json",
        help="Path to write JSON output (default: output/output.json)",
    )
    args = parser.parse_args()

    work_dir = args.dir
    cloned_dir = ""
    pkg = args.pkg

    if args.repo:
        cloned_dir = GitSupport.clone_repo(args.repo, args.ref or None)
        work_dir = cloned_dir
        if not pkg:
            pkg = GitSupport.pkg_from_repo_url(args.repo)

    print(f"Extracting from: {os.path.basename(work_dir)}")
    res, elapsed = extract(pkg, work_dir)
    print("Code extraction complete.")
    print(f"Total time elapsed: {elapsed}")

    out_path = args.out
    out_dir = os.path.dirname(out_path) or "."
    os.makedirs(out_dir, exist_ok=True)
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(res, f, indent=2)
    print(f"Wrote results to {out_path}")

    if cloned_dir and not args.keep_clone:
        shutil.rmtree(cloned_dir, ignore_errors=True)  


if __name__ == "__main__":
    extract_cli()