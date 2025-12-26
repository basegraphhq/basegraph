from enum import Enum
from typing import Optional, Union, Dict

class RelType(str, Enum):
    """Canonical relation type names for codegraph edges.

    Subclassing `str` makes serialization to JSON straightforward:
    `RelType.CALLS.value` -> "CALLS".
    """
    CLASS_DEF = "CLASS_DEF"
    FUNCTION_DEF = "FUNCTION_DEF"
    IMPORTS = "IMPORTS"
    PARAM_OF = "PARAM_OF"
    RETURNS = "RETURNS"
    CALLS = "CALLS"
    CALLED_BY = "CALLED_BY"
    YIELDS = "YIELDS"
    ASSIGNS = "ASSIGNS"
    INHERITS_FROM = "INHERITS_FROM"
    DECORATED_BY = "DECORATED_BY"
    TRY = "TRY"
    EXCEPT = "EXCEPT"
    FINALLY = "FINALLY"
    # Add additional relation types here as needed.

# Optional convenience mapping: node kind -> default relation type.
# Use this when it makes sense to infer a relation from the AST node kind.
KIND_TO_REL: Dict[str, RelType] = {
    "class": RelType.CLASS_DEF,
    "function": RelType.FUNCTION_DEF,
    "async_function": RelType.FUNCTION_DEF,
    # extend if you want other implicit mappings
}

def rel_to_str(rel: Optional[Union[RelType, str]]) -> Optional[str]:
    """Normalize a RelType or plain-string into a canonical string or None."""
    if rel is None:
        return None
    if isinstance(rel, RelType):
        return rel.value
    return str(rel)