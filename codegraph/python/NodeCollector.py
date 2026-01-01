import ast
from typing import Dict, Any, Optional, List, Union
from rel_types import RelType, KIND_TO_REL, rel_to_str
from ast_utils import extract_docstring

def _get_node_kind(node: ast.AST) -> str:
    kind_map = {
        ast.ClassDef: "class",
        ast.FunctionDef: "function",
        ast.AsyncFunctionDef: "async_function",
        ast.Return: "returns",
        ast.Yield: "yields",
        ast.YieldFrom: "yields",
        ast.Import: "import",
        ast.ImportFrom: "import",
        ast.Assign: "assignment",
        ast.AugAssign: "augmented_assignment",
        ast.Try: "try",
        ast.ExceptHandler: "except",
    }
    return kind_map.get(type(node), "skip_node")


class NodeCollector(ast.NodeVisitor):
    def __init__(
        self,
        module_name: Optional[str] = None,
        source_file: Optional[str] = None,
        source_lines: Optional[List[str]] = None,
        filepath: Optional[str] = None,
    ):
        self.module_name = module_name
        self.source_file = source_file
        self.source_lines = source_lines or []
        self.filepath = filepath
        self.scope_stack: list[str] = []
        self.scope_kinds: list[str] = []
        self.result: Dict[str, Dict[str, Any]] = {}
        # map local symbol -> fully-qualified symbol from imports (e.g. foo -> pkg.module.foo)
        self._import_map: Dict[str, str] = {}

        # NEW: counter for synthetic nodes so keys don't need line/col
        self._synthetic_counters: Dict[tuple[str, str], int] = {}

    # ---------- helpers ----------

    def _extract_code(self, start_line: Optional[int], end_line: Optional[int]) -> Optional[str]:
        """Extract source code lines between start and end (1-indexed, inclusive)."""
        if not self.source_lines or start_line is None or end_line is None:
            return None
        # Convert to 0-indexed
        start_idx = max(0, start_line - 1)
        end_idx = min(len(self.source_lines), end_line)
        if start_idx >= end_idx:
            return None
        return "\n".join(self.source_lines[start_idx:end_idx])

    def _make_base_meta(
        self,
        qualified_name: str,
        parent_qualified_name: Optional[str],
        *,
        name: Optional[str] = None,
        kind: Optional[str] = None,
        ast_type: Optional[str] = None,
        pos: Optional[Dict[str, Optional[int]]] = None,
        annotation: Optional[str] = None,
        modifier: Optional[str] = None,
    ) -> Dict[str, Any]:
        """Return canonical meta dict with all canonical fields present."""
        if pos is None:
            pos = {"start": None, "end": None}

        # Extract code snippet from source lines
        code = self._extract_code(pos.get("start"), pos.get("end"))

        return {
            "name": name,
            "kind": kind,
            "ast_type": ast_type,
            "pos": pos,
            "qualified_name": qualified_name,
            "parent_qualified_name": parent_qualified_name,
            "annotation": annotation,
            "modifier": modifier,
            "docstring": None,
            "relations": [],
            "code": code,
            "filepath": self.filepath,
            "language": "python",
        }


    def _current_scope_qual(self) -> Optional[str]:
        """Return qualname of current scope, or None at top level."""
        return self.scope_stack[-1] if self.scope_stack else None

    def _make_qualname(self, name: str) -> str:
        """Build a qualified name for a new named scope."""
        parent = self._current_scope_qual()
        if parent is None:
            if self.module_name:
                return f"{self.module_name}.{name}"
            return name
        return f"{parent}.{name}"

    def _pos_dict(self, node: ast.AST) -> Dict[str, Optional[int]]:
        return {
            "start": getattr(node, "lineno", None),
            "end": getattr(node, "end_lineno", None),
        }

    def _current_scope_kind(self) -> Optional[str]:
        return self.scope_kinds[-1] if self.scope_kinds else None

    def _current_class_scope(self) -> Optional[str]:
        for qual, kind in reversed(list(zip(self.scope_stack, self.scope_kinds))):
            if kind == "class":
                return qual
        return None

    def _current_callable_scope(self) -> Optional[str]:
        for qual, kind in reversed(list(zip(self.scope_stack, self.scope_kinds))):
            if kind in {"function", "async_function"}:
                return qual
        return None

    def _push_scope(self, qual: str, kind: str):
        self.scope_stack.append(qual)
        self.scope_kinds.append(kind)

    def _pop_scope(self):
        if self.scope_stack:
            self.scope_stack.pop()
        if self.scope_kinds:
            self.scope_kinds.pop()

    def _expr_to_name(self, expr: Optional[ast.AST]) -> Optional[str]:
        """Return a compact, readable name for an expression, or None."""
        if expr is None:
            return None
        # simple name or attribute
        try:
            if isinstance(expr, ast.Name):
                return expr.id
            if isinstance(expr, ast.Attribute):
                return self._extract_call_name(expr)
            # constants: show value shorthand
            if isinstance(expr, ast.Constant):
                return repr(expr.value)
            # subsume calls, subscripts, etc. into an unparse fallback
            try:
                return ast.unparse(expr)
            except Exception:
                return None
        except Exception:
            return None
        
    def _make_synthetic_key(self, node: ast.AST, kind: str) -> str:
        """
        Create a stable key for unnamed nodes.

        Strategy:
        - If the node has an obvious name (Assign/AugAssign) use that name.
        - For Return nodes use the returned-expression text when available, or "null" if returning nothing.
        - For Yield nodes use "yield".
        - Otherwise fall back to the node's line number for stability.
        """
        parent = self._current_scope_qual() or self.module_name or self.source_file or "<unknown>"

        # Try to extract a name-like part
        try:
            if isinstance(node, ast.Assign):
                for t in node.targets:
                    if isinstance(t, ast.Name):
                        return f"{parent}.{kind}.{t.id}"
            if isinstance(node, ast.AugAssign):
                if isinstance(node.target, ast.Name):
                    return f"{parent}.{kind}.{node.target.id}"
            if isinstance(node, ast.Return):
                # prefer the returned-expression string; use literal "null" when no value
                name_part = self._expr_to_name(node.value)
                if name_part is None:
                    if node.value is None:
                        name_part = "null"
                    else:
                        # fallback to line number if we couldn't name the expression
                        name_part = str(getattr(node, "lineno", 0))
                return f"{parent}.{kind}.{name_part}"
            if isinstance(node, (ast.Yield, ast.YieldFrom)):
                return f"{parent}.{kind}.yield"
        except Exception:
            # Silently fall through to lineno-based fallback if name extraction fails.
            pass

        lineno = getattr(node, "lineno", None) or 0
        return f"{parent}.{kind}.{lineno}"


    def _set_relation(self, meta: Dict[str, Any], source: Optional[str], rel_type: Optional[Union[RelType, str]], target: Optional[str], pos: Optional[Dict[str, Optional[int]]] = None):
        """Append a relation to the meta's relations list."""
        if not source or not target or rel_type is None:
            return
        rel_str = rel_to_str(rel_type)
        if rel_str is None:
            return
        meta["relations"].append({
            "source": source,
            "rel_type": rel_str,
            "target": target,
            "pos": pos or {"start": None, "end": None},
        })




    # ---------- recording helpers ----------

    def _record_named_node(self, node: ast.AST, kind: str):
        raw_name = getattr(node, "name", None)
        if raw_name is None:
            return

        qualified_name = self._make_qualname(raw_name)
        parent_qual = self._current_scope_qual() or self.module_name

        meta = self._make_base_meta(
            qualified_name=qualified_name,
            parent_qualified_name=parent_qual,
            name=raw_name,
            kind=kind,
            ast_type=type(node).__name__,
            pos=self._pos_dict(node),
            annotation=None,
            modifier=None,
        )

        meta["docstring"] = extract_docstring(node)

        rel_type = None
        if kind == "class":
            rel_type = RelType.CLASS_DEF
        elif kind in {"function", "async_function"}:
            rel_type = RelType.FUNCTION_DEF

        if rel_type:
            self._set_relation(meta, source=qualified_name, rel_type=rel_type, target=parent_qual, pos=meta["pos"])

        self.result[qualified_name] = meta



    def _record_unnamed_node(self, node: ast.AST, kind: str):
        if kind == "skip_node":
            return
        parent_qual = self._current_scope_qual() or self.module_name
        key = self._make_synthetic_key(node, kind)
        meta = self._make_base_meta(
            qualified_name=key,
            parent_qualified_name=parent_qual,
            name=None,
            kind=kind,
            ast_type=type(node).__name__,
            pos=self._pos_dict(node),
            annotation=None,
            modifier=None,
        )
        self.result[key] = meta


    def _record_import_node(self, node: ast.AST, module: str, alias: Optional[str]):
        name = alias or module
        # Use qualified name so the key is file-prefixed / scope-prefixed
        key = self._make_qualname(name)
        parent_qual = self._current_scope_qual() or self.module_name

        meta = self._make_base_meta(
            qualified_name=key,
            parent_qualified_name=parent_qual,
            name=name,
            kind="import",
            ast_type=type(node).__name__,
            pos=self._pos_dict(node),
            annotation=None,
            modifier=None,
        )
        self._set_relation(meta, source=key, rel_type=RelType.IMPORTS, target=parent_qual, pos=meta["pos"])
        self.result[key] = meta
        # Populate import map to help resolve simple names to fully-qualified targets.
        # `module` may be full (e.g. "pkg.mod.name") for `from pkg.mod import name` calls
        # or may be a module path for plain imports. Map the local symbol (alias or last part)
        # to the full module path so later call resolution can expand names.
        try:
            # `full` is the module path for plain imports (e.g., "pkg.mod" for `import pkg.mod`)
            # or the fully-qualified symbol (e.g., "pkg.mod.func" for `from pkg.mod import func`)
            full = module
            # For plain imports like `import pkg.mod`, Python binds only the top-level package `pkg`.
            # We map `pkg` -> `pkg` to preserve this binding for attribute resolution.
            # For `from` imports, we map the imported symbol to its fully-qualified name.
            if isinstance(node, ast.Import):
                if alias:
                    # `import pkg.mod as pm` -> map `pm` to `pkg.mod`
                    local = alias
                    self._import_map[local] = full
                else:
                    # `import pkg.mod` -> map `pkg` to `pkg` (top-level package)
                    top = module.split(".")[0]
                    if top not in self._import_map:
                        self._import_map[top] = top
            else:
                # For `from` imports, map the imported name to the full path
                local = alias or module.split(".")[-1]
                if local:
                    self._import_map[local] = full
        except Exception:
            # Best-effort population of the import map; failures here should not break collection.
            pass



    def _record_param_node(self, func_qual: str, arg: ast.arg, modifier: str = ""):
        param_name = getattr(arg, "arg", None)
        if not param_name:
            return
        key = f"{func_qual}.param.{param_name}"
        annotation = None
        if getattr(arg, "annotation", None) is not None:
            try:
                annotation = ast.unparse(arg.annotation)
            except Exception:
                annotation = None

        meta = self._make_base_meta(
            qualified_name=key,
            parent_qualified_name=func_qual,
            name=param_name,
            kind="params_of",
            ast_type="arg",
            pos={"start": getattr(arg, "lineno", None), "end": getattr(arg, "end_lineno", None)},
            annotation=annotation,
            modifier=modifier or None,
        )
        self._set_relation(meta, source=key, rel_type=RelType.PARAM_OF, target=func_qual, pos=meta["pos"])
        self.result[key] = meta


    def _record_return_node(self, func_qual: str, node: ast.AST):
        if getattr(node, "returns", None) is None:
            return
        try:
            return_type = ast.unparse(node.returns)
        except Exception:
            return_type = None

        key = f"{func_qual}.return"
        meta = self._make_base_meta(
            qualified_name=key,
            parent_qualified_name=func_qual,
            name=return_type,
            kind="returns",
            ast_type=type(node.returns).__name__ if node.returns is not None else "NoneType",
            pos=self._pos_dict(node.returns),
            annotation=None,
            modifier=None,
        )
        self._set_relation(meta, source=key, rel_type=RelType.RETURNS, target=func_qual, pos=meta["pos"])
        self.result[key] = meta


    def _record_function_relationships(self, func_qual: str):
        # Intentionally minimal: no extra relationships beyond definition edges
        return

    def _extract_call_name(self, func: ast.AST) -> Optional[str]:
        # Name: try to expand using import map (e.g. `from pkg.mod import f` -> f -> pkg.mod.f)
        if isinstance(func, ast.Name):
            name = func.id
            mapped = self._import_map.get(name)
            if mapped:
                return mapped
            return name
        # Attribute: build dotted name and substitute import map for base if available
        if isinstance(func, ast.Attribute):
            parts: List[str] = []
            current = func
            while isinstance(current, ast.Attribute):
                parts.append(current.attr)
                current = current.value
            base: Optional[str] = None
            if isinstance(current, ast.Name):
                base = current.id
            else:
                try:
                    base = ast.unparse(current)
                except Exception:
                    base = None

            if base in {"self", "cls"}:
                class_scope = self._current_class_scope()
                if class_scope:
                    base = class_scope
            if base is None:
                return None
            # expand base using import map if present
            if base in self._import_map:
                base = self._import_map[base]
            parts.append(base)
            parts.reverse()
            return ".".join(parts)
        # fallback: try to unparse complex expressions
        try:
            text = ast.unparse(func)
            # If the unparsed text is a simple name, try map as well
            if "." not in text:
                mapped = self._import_map.get(text)
                if mapped:
                    return mapped
            return text
        except Exception:
            return None

    # ---------- named scopes ----------

    def visit_ClassDef(self, node: ast.ClassDef):
        kind = _get_node_kind(node)  # "class"
        self._record_named_node(node, kind)

        qual = self._make_qualname(node.name)
        # Add inheritance relations
        for base in node.bases:
            base_name = self._extract_call_name(base)
            if base_name:
                self._set_relation(self.result[qual], source=qual, rel_type=RelType.INHERITS_FROM, target=base_name, pos=self._pos_dict(base))
        # Add decorator relations
        for decorator in node.decorator_list:
            decorator_name = self._extract_call_name(decorator)
            if decorator_name:
                self._set_relation(self.result[qual], source=qual, rel_type=RelType.DECORATED_BY, target=decorator_name, pos=self._pos_dict(decorator))
        self._push_scope(qual, kind)
        self.generic_visit(node)
        self._pop_scope()

    def visit_FunctionDef(self, node: ast.FunctionDef):
        kind = _get_node_kind(node)  # "function"
        self._record_named_node(node, kind)

        qual = self._make_qualname(node.name)
        self._record_function_relationships(qual)
        for arg in node.args.posonlyargs:
            self._record_param_node(qual, arg, modifier="posonly")
        for arg in node.args.args:
            self._record_param_node(qual, arg)
        if node.args.vararg:
            self._record_param_node(qual, node.args.vararg, modifier="vararg")
        for arg in node.args.kwonlyargs:
            self._record_param_node(qual, arg, modifier="kwonly")
        if node.args.kwarg:
            self._record_param_node(qual, node.args.kwarg, modifier="kwarg")
        self._record_return_node(qual, node)
        # Add decorator relations
        for decorator in node.decorator_list:
            decorator_name = self._extract_call_name(decorator)
            if decorator_name:
                self._set_relation(self.result[qual], source=qual, rel_type=RelType.DECORATED_BY, target=decorator_name, pos=self._pos_dict(decorator))
        self._push_scope(qual, kind)
        self.generic_visit(node)
        self._pop_scope()

    def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef):
        kind = _get_node_kind(node)  # "async_function"
        self._record_named_node(node, kind)

        qual = self._make_qualname(node.name)
        self._record_function_relationships(qual)
        for arg in node.args.posonlyargs:
            self._record_param_node(qual, arg, modifier="posonly")
        for arg in node.args.args:
            self._record_param_node(qual, arg)
        if node.args.vararg:
            self._record_param_node(qual, node.args.vararg, modifier="vararg")
        for arg in node.args.kwonlyargs:
            self._record_param_node(qual, arg, modifier="kwonly")
        if node.args.kwarg:
            self._record_param_node(qual, node.args.kwarg, modifier="kwarg")
        self._record_return_node(qual, node)
        # Add decorator relations
        for decorator in node.decorator_list:
            decorator_name = self._extract_call_name(decorator)
            if decorator_name:
                self._set_relation(self.result[qual], source=qual, rel_type=RelType.DECORATED_BY, target=decorator_name, pos=self._pos_dict(decorator))
        self._push_scope(qual, kind)
        self.generic_visit(node)
        self._pop_scope()

    # ---------- assignments ----------

    def _iter_assign_targets(self, target: ast.expr):
        """
        Yield all simple Name nodes contained in an assignment target.

        This covers plain names (e.g., `x = 1`) as well as names that
        appear inside tuple/list unpacking and starred targets
        (e.g., `a, (b, c) = values`, `*rest, last = seq`).
        """
        if isinstance(target, ast.Name):
            yield target
        elif isinstance(target, (ast.Tuple, ast.List)):
            for elt in target.elts:
                yield from self._iter_assign_targets(elt)
        elif isinstance(target, ast.Starred):
            # For starred targets like `*rest`, recurse into the value.
            yield from self._iter_assign_targets(target.value)

    def visit_Assign(self, node: ast.Assign):
        parent = self._current_scope_qual() or self.module_name
        for target in node.targets:
            for name_node in self._iter_assign_targets(target):
                key = f"{parent}.assignment.{name_node.id}"
                meta = self._make_base_meta(
                    qualified_name=key,
                    parent_qualified_name=parent,
                    name=name_node.id,
                    kind="assignment",
                    ast_type="Assign",
                    pos=self._pos_dict(node),
                )
                self._set_relation(
                    meta,
                    source=key,
                    rel_type=RelType.ASSIGNS,
                    target=parent,
                    pos=meta["pos"],
                )
                self.result[key] = meta
        self.generic_visit(node)

    def visit_AugAssign(self, node: ast.AugAssign):
        if isinstance(node.target, ast.Name):
            parent = self._current_scope_qual() or self.module_name
            key = f"{parent}.augmented_assignment.{node.target.id}"
            meta = self._make_base_meta(
                qualified_name=key,
                parent_qualified_name=self._current_scope_qual() or self.module_name,
                name=node.target.id,
                kind="augmented_assignment",
                ast_type="AugAssign",
                pos=self._pos_dict(node),
            )
            self._set_relation(meta, source=key, rel_type=RelType.ASSIGNS, target=self._current_scope_qual() or self.module_name, pos=meta["pos"])
            self.result[key] = meta
        self.generic_visit(node)

    # ---------- control-flow statements ----------

    def visit_For(self, node: ast.For):
        kind = _get_node_kind(node)  # currently "skip_node" unless added to kind_map
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_AsyncFor(self, node: ast.AsyncFor):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_While(self, node: ast.While):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_If(self, node: ast.If):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_Try(self, node: ast.Try):
        """
        Record a synthetic 'try' node and one 'except' node per handler.
        Except keys follow: <parent>.except.<handler_ordinal>.<exception_name>
        Relations:
        - try_node --EXCEPT--> except_node
        - except_node --TRY--> try_node
        """
        parent_qual = self._current_scope_qual() or self.module_name
        # create try node (try_key remains based on synthetic key which already uses parent)
        try_key = self._make_synthetic_key(node, "try")
        try_meta = self._make_base_meta(
            qualified_name=try_key,
            parent_qualified_name=parent_qual,
            name=None,
            kind="try",
            ast_type=type(node).__name__,
            pos=self._pos_dict(node),
        )
        self.result[try_key] = try_meta

        for idx, handler in enumerate(node.handlers):
            # readable exception identifier: type name or 'any'
            exc_name = None
            try:
                exc_name = self._expr_to_name(handler.type) if getattr(handler, "type", None) is not None else None
            except Exception:
                exc_name = None
            if not exc_name:
                exc_name = getattr(handler, "name", None) or "any"

            # Use parent + ordinal + exception-name for deterministic, human-readable keys
            except_key = f"{parent_qual}.except.{idx}.{exc_name}"

            except_meta = self._make_base_meta(
                qualified_name=except_key,
                parent_qualified_name=parent_qual,
                name=exc_name,
                kind="except",
                ast_type=type(handler).__name__,
                pos=self._pos_dict(handler),
            )

            # relations: try -> EXCEPT -> except_node
            self._set_relation(try_meta, source=try_key, rel_type=RelType.EXCEPT, target=except_key, pos=self._pos_dict(handler))
            # except -> TRY -> try_node
            self._set_relation(except_meta, source=except_key, rel_type=RelType.TRY, target=try_key, pos=self._pos_dict(handler))

            self.result[except_key] = except_meta

        # create finally node if present
        if node.finalbody:
            # Derive a stable line number for the synthetic 'finally' node.
            # Prefer the Try node's end position, then fall back to the first
            # statement in the finalbody, and finally to the Try's own lineno.
            lineno = getattr(node, "end_lineno", None)
            if lineno is None:
                lineno = getattr(node.finalbody[0], "lineno", None)
            if lineno is None:
                lineno = getattr(node, "lineno", None) or 0
            # key the finally node under the parent with line number for uniqueness
            finally_key = f"{parent_qual}.finally.{lineno}"
            finally_meta = self._make_base_meta(
                qualified_name=finally_key,
                parent_qualified_name=parent_qual,
                name=None,
                kind="finally",
                ast_type="Finally",
                pos=self._pos_dict(node.finalbody[0]) if node.finalbody else self._pos_dict(node),
            )
            # relations: try -> FINALLY -> finally_node
            self._set_relation(try_meta, source=try_key, rel_type=RelType.FINALLY, target=finally_key, pos=self._pos_dict(node))
            # finally -> TRY -> try_node (reverse link)
            self._set_relation(finally_meta, source=finally_key, rel_type=RelType.TRY, target=try_key, pos=self._pos_dict(node))
            self.result[finally_key] = finally_meta

        # continue walking children (body, handlers' bodies, orelse, finalbody)
        self.generic_visit(node)

    def visit_With(self, node: ast.With):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_AsyncWith(self, node: ast.AsyncWith):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    # ---------- expressions / comprehensions / lambdas ----------

    def visit_ListComp(self, node: ast.ListComp):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_DictComp(self, node: ast.DictComp):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_SetComp(self, node: ast.SetComp):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_GeneratorExp(self, node: ast.GeneratorExp):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_Lambda(self, node: ast.Lambda):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    # ---------- imports ----------

    def visit_Import(self, node: ast.Import):
        for alias in node.names:
            self._record_import_node(node, alias.name, alias.asname)
        self.generic_visit(node)

    def visit_ImportFrom(self, node: ast.ImportFrom):
        module = node.module or ""
        for alias in node.names:
            target = f"{module}.{alias.name}" if module else alias.name
            self._record_import_node(node, target, alias.asname)
        self.generic_visit(node)

    # ---------- simple statements / flow control ----------

    def visit_Return(self, node: ast.Return):
        kind = _get_node_kind(node)
        if kind == "returns":
            func_qual = self._current_callable_scope()
            # compute a key that includes the returned-expression (or "null")
            key = self._make_synthetic_key(node, kind)
            returned_name = self._expr_to_name(node.value)  # None when no value or not nameable
            meta = self._make_base_meta(
                qualified_name=key,
                parent_qualified_name=func_qual or self.module_name,
                name=returned_name,
                kind=kind,
                ast_type=type(node).__name__,
                pos=self._pos_dict(node),
                annotation=None,
                modifier=None,
            )
            self._set_relation(meta, source=key, rel_type=RelType.RETURNS, target=func_qual, pos=meta["pos"])
            self.result[key] = meta
        self.generic_visit(node)

    def visit_Yield(self, node: ast.Yield):
        kind = _get_node_kind(node)
        if kind == "yields":
            func_qual = self._current_callable_scope()
            key = self._make_synthetic_key(node, kind)
            yielded_name = self._expr_to_name(node.value)  # None when no value or not nameable
            meta = self._make_base_meta(
                qualified_name=key,
                parent_qualified_name=func_qual or self.module_name,
                name=yielded_name,
                kind=kind,
                ast_type=type(node).__name__,
                pos=self._pos_dict(node),
                annotation=None,
                modifier=None,
            )
            self._set_relation(meta, source=key, rel_type=RelType.YIELDS, target=func_qual, pos=meta["pos"])
            self.result[key] = meta
        self.generic_visit(node)


    def visit_YieldFrom(self, node: ast.YieldFrom):
        kind = _get_node_kind(node)
        if kind == "yields":
            func_qual = self._current_callable_scope()
            key = self._make_synthetic_key(node, kind)
            yielded_name = self._expr_to_name(node.value)  # None when no value or not nameable
            meta = self._make_base_meta(
                qualified_name=key,
                parent_qualified_name=func_qual or self.module_name,
                name=yielded_name,
                kind=kind,
                ast_type=type(node).__name__,
                pos=self._pos_dict(node),
                annotation=None,
                modifier=None,
            )
            self._set_relation(meta, source=key, rel_type=RelType.YIELDS, target=func_qual, pos=meta["pos"])
            self.result[key] = meta
        self.generic_visit(node)

    def visit_Raise(self, node: ast.Raise):
        kind = _get_node_kind(node)  # currently "skip_node"
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_Assert(self, node: ast.Assert):
        kind = _get_node_kind(node)  # currently "skip_node"
        self._record_unnamed_node(node, kind)
        self.generic_visit(node)

    def visit_Break(self, node: ast.Break):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        # no children to visit

    def visit_Continue(self, node: ast.Continue):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        # no children to visit

    def visit_Pass(self, node: ast.Pass):
        kind = _get_node_kind(node)
        self._record_unnamed_node(node, kind)
        # no children to visit

    def visit_Call(self, node: ast.Call):
        caller = self._current_callable_scope()
        callee = self._extract_call_name(node.func)
        if caller and callee:
            pos = self._pos_dict(node)
            self._set_relation(self.result[caller], source=caller, rel_type=RelType.CALLS, target=callee, pos=pos)
            if callee in self.result:
                self._set_relation(self.result[callee], source=callee, rel_type=RelType.CALLED_BY, target=caller, pos=pos)
        self.generic_visit(node)