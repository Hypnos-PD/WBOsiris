#!/usr/bin/env python3
"""Convert AssetStudio text dumps into the small runtime schema used by the web preview."""
import json
import re
from pathlib import Path

SOURCE = Path("/tmp/opencode/status-prefab-dumps")
OUTPUT = Path("web/src/statusPrefabData.json")

def number(text, name, default=0.0):
    match = re.search(rf"(?:float|int|SInt16|UInt8|UInt16) {re.escape(name)} = ([^\r\n]+)", text)
    if not match:
        return default
    try:
        return float(match.group(1))
    except ValueError:
        return default

def integer(text, name, default=0):
    return int(number(text, name, default))

def path_id(text, kind):
    match = re.search(rf"PPtr<{kind}> m_GameObject[\s\S]*?SInt64 m_PathID = (-?\d+)", text)
    return int(match.group(1)) if match else None

def asset_name(path):
    return re.sub(r" @-?\d+\.txt$", "", path.name)

def material_properties(text):
    textures = {}
    for match in re.finditer(r'string first = "([^\"]+)"\s+UnityTexEnv second[\s\S]*?SInt64 m_PathID = (-?\d+)[\s\S]*?m_Scale[\s\S]*?float x = ([^\r\n]+)[\s\S]*?float y = ([^\r\n]+)[\s\S]*?m_Offset[\s\S]*?float x = ([^\r\n]+)[\s\S]*?float y = ([^\r\n]+)', text):
        name, asset, sx, sy, ox, oy = match.groups()
        textures[name] = {"asset": int(asset), "scale": [float(sx), float(sy)], "offset": [float(ox), float(oy)]}
    floats = {name: float(value) for name, value in re.findall(r'string first = "([^\"]+)"\s+float second = ([^\r\n]+)', text)}
    colors = {}
    for match in re.finditer(r'string first = "([^\"]+)"\s+ColorRGBA second\s+float r = ([^\r\n]+)\s+float g = ([^\r\n]+)\s+float b = ([^\r\n]+)\s+float a = ([^\r\n]+)', text):
        name, *values = match.groups()
        colors[name] = [float(value) for value in values]
    keywords = re.findall(r'string data = "([^\"]+)"', text.split("vector m_InvalidKeywords", 1)[0])
    return textures, floats, colors, keywords

def parse_prefab(folder):
    files = list(folder.glob("*.txt"))
    names = {int(match.group(1)): asset_name(path) for path in files if (match := re.search(r" @(-?\d+)\.txt$", path.name))}
    objects = {path: path.read_text(errors="replace") for path in files}
    nodes = {}
    transforms = {}
    asset_kinds = {}
    systems = []
    renderers = []
    materials = {}
    for path, text in objects.items():
        id_match = re.search(r" @(-?\d+)\.txt$", path.name)
        object_id = int(id_match.group(1)) if id_match else None
        if object_id is not None and text.startswith("Texture2D Base"):
            asset_kinds[str(object_id)] = "Texture2D"
        elif object_id is not None and text.startswith("Mesh Base"):
            asset_kinds[str(object_id)] = "Mesh"
        if text.startswith("GameObject Base"):
            game_name = re.search(r"string m_Name = \"([^\"]+)\"", text)
            if game_name:
                components = [int(value) for value in re.findall(r"PPtr<Component>[\s\S]*?SInt64 m_PathID = (-?\d+)", text)]
                nodes[object_id] = {"id": object_id, "name": game_name.group(1), "components": components}
        elif text.startswith("Transform Base"):
            rotation_match = re.search(r"m_LocalRotation[\s\S]*?float x = ([^\r\n]+)[\s\S]*?float y = ([^\r\n]+)[\s\S]*?float z = ([^\r\n]+)[\s\S]*?float w = ([^\r\n]+)", text)
            rotation = [float(value) for value in rotation_match.groups()] if rotation_match else [0, 0, 0, 1]
            position_match = re.search(r"m_LocalPosition[\s\S]*?float x = ([^\r\n]+)[\s\S]*?float y = ([^\r\n]+)[\s\S]*?float z = ([^\r\n]+)", text)
            pos = [float(value) for value in position_match.groups()] if position_match else [0, 0, 0]
            scale_match = re.search(r"m_LocalScale[\s\S]*?float x = ([^\r\n]+)[\s\S]*?float y = ([^\r\n]+)[\s\S]*?float z = ([^\r\n]+)", text)
            scale = [float(value) for value in scale_match.groups()] if scale_match else [1, 1, 1]
            children = [int(value) for value in re.findall(r"m_Children[\s\S]*?PPtr<Transform> data[\s\S]*?SInt64 m_PathID = (-?\d+)", text)]
            father = re.search(r"m_Father[\s\S]*?SInt64 m_PathID = (-?\d+)", text)
            transforms[object_id] = {"position": pos, "rotation": rotation, "scale": scale, "children": children, "father": int(father.group(1)) if father else 0}
        elif text.startswith("ParticleSystem Base"):
            game_id = path_id(text, "GameObject")
            if game_id is not None:
                systems.append({"id": object_id, "gameObject": game_id, "duration": number(text, "lengthInSec", 1), "looping": "looping = True" in text, "lifetime": number(text, "scalar", 1), "maxParticles": integer(text, "maxNumParticles"), "startSize": [number(text, "scalar", 1), number(text, "scalar", 1)], "startSpeed": number(text, "scalar"), "rate": number(text, "scalar"), "bursts": integer(text, "m_BurstCount")})
        elif text.startswith("ParticleSystemRenderer Base"):
            game_id = path_id(text, "GameObject")
            if game_id is not None:
                mesh = re.search(r"PPtr<Mesh> m_Mesh[\s\S]*?SInt64 m_PathID = (-?\d+)", text)
                material = re.search(r"PPtr<Material> data[\s\S]*?SInt64 m_PathID = (-?\d+)", text)
                renderers.append({"id": object_id, "gameObject": game_id, "enabled": "m_Enabled = True" in text, "mode": integer(text, "m_RenderMode"), "alignment": integer(text, "m_RenderAlignment"), "order": integer(text, "m_SortingOrder"), "fudge": number(text, "m_SortingFudge"), "mesh": int(mesh.group(1)) if mesh else 0, "material": int(material.group(1)) if material else 0})
        elif text.startswith("Material Base"):
            material_name = re.search(r"string m_Name = \"([^\"]+)\"", text)
            shader = re.search(r"PPtr<Shader> m_Shader[\s\S]*?SInt64 m_PathID = (-?\d+)", text)
            if material_name:
                textures, floats, colors, keywords = material_properties(text)
                base = textures.get("_BaseMap", textures.get("_MainTex", {})).get("asset", 0)
                materials[object_id] = {"name": material_name.group(1), "shader": int(shader.group(1)) if shader else 0, "base": base, "textures": textures, "floats": floats, "colors": colors, "keywords": keywords}
    return {"name": folder.name, "nodes": list(nodes.values()), "transforms": transforms, "systems": systems, "renderers": renderers, "materials": materials, "assets": names, "assetKinds": asset_kinds}

OUTPUT.parent.mkdir(parents=True, exist_ok=True)
OUTPUT.write_text(json.dumps({folder.name: parse_prefab(folder) for folder in sorted(SOURCE.iterdir()) if folder.is_dir()}, ensure_ascii=False, indent=2) + "\n")
