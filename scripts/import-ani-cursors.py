"""Export 32-bit ANI cursors as transparent, pixel-aligned web sprite sheets.

Usage: python scripts/import-ani-cursors.py SOURCE OUTPUT --size 52
Requires Pillow. Reads the resource files only; never executes install.inf.
The date outfit is drawn at 52 px and supplied as a 4x (208 px) enlargement.
"""

import argparse
import io
import json
import math
import struct
from pathlib import Path

from PIL import Image


SEMANTICS = {
    "Normal": ("default", "默认指针"),
    "Alternate": ("default", "备用指针（向上箭头）"),
    "Link": ("pointer", "悬停可点击元素"),
    "Person": ("pointer", "悬停人物/话题"),
    "Pin": ("pointer", "悬停可固定元素"),
    "Text": ("text", "文本输入区"),
    "Handwriting": ("text", "手写/编辑区"),
    "Move": ("move", "拖拽移动"),
    "Horizontal": ("ew-resize", "左右拉伸"),
    "Vertical": ("ns-resize", "上下拉伸"),
    "Diagonal1": ("nwse-resize", "左上↔右下拉伸"),
    "Diagonal2": ("nesw-resize", "右上↔左下拉伸"),
    "Precision": ("crosshair", "精确选择/十字定位"),
    "Help": ("help", "帮助/疑问"),
    "Working": ("progress", "后台处理中"),
    "Busy": ("wait", "忙碌/等待"),
    "Unavailable": ("not-allowed", "禁止操作"),
}


def chunks(data, start=0):
    while start < len(data):
        if start + 8 > len(data):
            raise ValueError("Truncated RIFF chunk header")
        tag, length = struct.unpack_from("<4sI", data, start)
        end = start + 8 + length
        if end > len(data):
            raise ValueError("Truncated RIFF chunk")
        yield tag, data[start + 8:end]
        start = end + (length & 1)


def decode_cursor(raw):
    reserved, kind, count = struct.unpack_from("<3H", raw)
    if reserved or kind != 2 or count < 1:
        raise ValueError("Expected a CUR frame with a click hotspot")
    _, _, _, _, hot_x, hot_y, length, offset = struct.unpack_from("<4B2H2I", raw, 6)
    bitmap = raw[offset:offset + length]
    if len(bitmap) != length:
        raise ValueError("Truncated cursor image")
    if bitmap.startswith(b"\x89PNG\r\n\x1a\n"):
        frame = Image.open(io.BytesIO(bitmap)).convert("RGBA")
    else:
        header, width, double_height, planes, bits, compression = struct.unpack_from("<IiiHHI", bitmap)
        if header < 40 or width <= 0 or double_height <= 0 or double_height % 2 or planes != 1 or bits != 32 or compression:
            raise ValueError("Only uncompressed 32-bit DIB cursors are supported")
        height = double_height // 2
        xor_end = header + width * height * 4
        frame = Image.frombytes("RGBA", (width, height), bitmap[header:xor_end], "raw", "BGRA", 0, -1)
        # Older CUR images may contain no alpha; their AND mask defines transparency.
        if frame.getchannel("A").getextrema() == (0, 0):
            stride = ((width + 31) // 32) * 4
            mask = bitmap[xor_end:xor_end + stride * height]
            if len(mask) != stride * height:
                raise ValueError("Missing cursor transparency mask")
            alpha = Image.new("L", frame.size)
            alpha.putdata([
                0 if mask[(height - 1 - y) * stride + x // 8] & (128 >> (x % 8)) else 255
                for y in range(height) for x in range(width)
            ])
            frame.putalpha(alpha)
    if not (0 <= hot_x < frame.width and 0 <= hot_y < frame.height):
        raise ValueError("Cursor hotspot is outside the image")
    return frame, (hot_x, hot_y)


def parse_ani(path):
    raw = path.read_bytes()
    if raw[:4] != b"RIFF" or raw[8:12] != b"ACON" or struct.unpack_from("<I", raw, 4)[0] + 8 != len(raw):
        raise ValueError(f"Invalid ANI file: {path.name}")
    header, frames, rates, sequence = None, [], None, None
    for tag, payload in chunks(raw, 12):
        if tag == b"anih":
            header = struct.unpack_from("<9I", payload)
        elif tag in (b"rate", b"seq "):
            values = list(struct.unpack("<" + "I" * (len(payload) // 4), payload))
            if tag == b"rate":
                rates = values
            else:
                sequence = values
        elif tag == b"LIST" and payload[:4] == b"fram":
            frames.extend(decode_cursor(frame) for kind, frame in chunks(payload, 4) if kind == b"icon")
    if not header or len(frames) != header[1] or not frames:
        raise ValueError(f"Missing ANI header or frames: {path.name}")
    sequence = sequence if header[8] & 2 else list(range(len(frames)))
    rates = rates if rates is not None else [header[7]] * header[2]
    if sequence is None or len(sequence) != header[2] or len(rates) != len(sequence) or any(rate < 1 for rate in rates):
        raise ValueError(f"Invalid ANI playback timing: {path.name}")
    # Expand unequal durations into equal steps without removing repeated frames.
    tick = math.gcd(*rates)
    timeline = [frames[index] for index, rate in zip(sequence, rates) for _ in range(rate // tick)]
    if len(timeline) > 200 or len({(frame.size, hotspot) for frame, hotspot in timeline}) != 1:
        raise ValueError("Animation exceeds web limits or has changing frame geometry")
    return timeline, tick * 1000 / 60


def build(source, output, size):
    if not 1 <= size <= 128:
        raise ValueError("Web cursor size must be between 1 and 128")
    bindings = {}
    for state, (css, _) in SEMANTICS.items():
        bindings.setdefault(css, []).append(state)
    manifest = {"source": source.parent.name, "cssBindings": bindings, "cursors": {}}
    (output / "sprite").mkdir(parents=True, exist_ok=True)
    for state, (css, label) in SEMANTICS.items():
        timeline, frame_ms = parse_ani(source / f"{state}.ani")
        original, hotspot = timeline[0]
        height = round(original.height * size / original.width)
        if height < 1 or height > 128:
            raise ValueError("Cursor height exceeds web limits")
        sheet = Image.new("RGBA", (size * len(timeline), height))
        for index, (frame, _) in enumerate(timeline):
            sheet.paste(frame.resize((size, height), Image.Resampling.NEAREST), (index * size, 0))
        sheet.save(output / "sprite" / f"{state}.png", optimize=True)
        manifest["cursors"][state] = {
            "sprite": f"sprite/{state}.png", "frames": len(timeline),
            "frameWidth": size, "frameHeight": height, "frameMs": frame_ms,
            "hotspot": [hotspot[0] * size / original.width, hotspot[1] * height / original.height],
            "css": css, "label": label, "animated": len(timeline) > 1,
        }
        print(f"{state}: {len(timeline)} frames, {size}x{height}, {frame_ms:g} ms")
    (output / "cursors.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--size", type=int, default=52)
    args = parser.parse_args()
    build(args.source, args.output, args.size)
