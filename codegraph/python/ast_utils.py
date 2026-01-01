"""Utility functions for AST operations."""
import ast
from typing import Optional


def extract_docstring(node_or_tree) -> Optional[str]:
    """
    Extract docstring from an AST node or tree.
    
    Works with both module trees (ast.Module) and nodes that have a body
    attribute (ast.ClassDef, ast.FunctionDef, etc.).
    
    Args:
        node_or_tree: An AST node or module tree
        
    Returns:
        The docstring as a string, or None if no docstring is found
    """
    # Handle nodes without a body attribute
    if not hasattr(node_or_tree, "body"):
        return None
    
    body = node_or_tree.body
    if not body:
        return None

    first_stmt = body[0]
    if not isinstance(first_stmt, ast.Expr):
        return None

    value = first_stmt.value
    if isinstance(value, ast.Constant) and isinstance(value.value, str):
        return value.value

    return None
