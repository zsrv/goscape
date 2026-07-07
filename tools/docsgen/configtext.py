"""Parser for RuneScript config text (content *.obj files, unpack all.* output)."""
import re

_HEADER = re.compile(r"^\[([^\]]+)\]\s*$")


def parse_config_text(text: str) -> list[dict[str, object]]:
    records: list[dict[str, object]] = []
    cur: dict[str, object] | None = None
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("//"):
            continue
        m = _HEADER.match(line)
        if m:
            cur = {"_debugname": m.group(1), "_index": len(records)}
            records.append(cur)
            continue
        if cur is None or "=" not in line:
            continue
        key, _, value = line.partition("=")
        key, value = key.strip(), value.strip()
        if key in cur:
            prev = cur[key]
            if isinstance(prev, list):
                prev.append(value)
            else:
                cur[key] = [prev, value]
        else:
            cur[key] = value
    return records
