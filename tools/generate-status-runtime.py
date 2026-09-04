#!/usr/bin/env python3
"""Generate status effect runtime data directly from Unity serialized objects.

Run with: uv run --with UnityPy python tools/generate-status-runtime.py
"""
import json
import math
from pathlib import Path

import UnityPy

UnityPy.config.FALLBACK_UNITY_VERSION = "2022.3.62f2"
SOURCE = Path("/home/aspharos/Data/wbunpacker_files/variants/Jpn/decrypted/Battle/Effect/Prefab")
OUTPUT = Path("web/src/statusRuntimeData.json")

def ptr(value):
    return int(getattr(value, "path_id", getattr(value, "m_PathID", 0)) or 0)

def vec(value, fields):
    return [float(getattr(value, field, 0)) for field in fields]

def curve(value):
    def finite(number):
        number = float(number)
        return number if math.isfinite(number) else 0
    def keys(item):
        return [[finite(key.time), finite(key.value), finite(key.inSlope), finite(key.outSlope)] for key in getattr(item, "m_Curve", [])]
    return {"mode": int(value.minMaxState), "max": finite(value.scalar), "min": finite(value.minScalar), "maxCurve": keys(value.maxCurve), "minCurve": keys(value.minCurve)}

def gradient(value):
    def one(item):
        colors = []
        for index in range(int(item.m_NumColorKeys)):
            color = getattr(item, f"key{index}")
            colors.append([float(getattr(item, f"ctime{index}")) / 65535, *vec(color, "rgba")])
        alphas = []
        for index in range(int(item.m_NumAlphaKeys)):
            color = getattr(item, f"key{index}")
            alphas.append([float(getattr(item, f"atime{index}")) / 65535, float(color.a)])
        return {"colors": colors, "alphas": alphas}
    return {"mode": int(value.minMaxState), "maxColor": vec(value.maxColor, "rgba"), "minColor": vec(value.minColor, "rgba"), "maxGradient": one(value.maxGradient), "minGradient": one(value.minGradient)}

def material(value):
    props = value.m_SavedProperties
    textures = {}
    for item in props.m_TexEnvs:
        name, env = item
        textures[name] = {"asset": ptr(env.m_Texture), "scale": vec(env.m_Scale, "xy"), "offset": vec(env.m_Offset, "xy")}
    return {"name": value.m_Name, "shader": ptr(value.m_Shader), "keywords": list(value.m_ValidKeywords), "textures": textures, "floats": {name: float(number) for name, number in props.m_Floats}, "colors": {name: vec(color, "rgba") for name, color in props.m_Colors}}

def system(value):
    initial, emission, shape = value.InitialModule, value.EmissionModule, value.ShapeModule
    bursts = []
    for burst in getattr(emission, "m_Bursts", []):
        bursts.append({"time": float(burst.time), "count": curve(burst.countCurve), "cycles": int(burst.cycleCount), "interval": float(burst.repeatInterval), "probability": float(burst.probability)})
    custom = value.CustomDataModule
    return {
        "duration": float(value.lengthInSec), "looping": bool(value.looping), "maxParticles": int(initial.maxNumParticles),
        "lifetime": curve(initial.startLifetime), "speed": curve(initial.startSpeed), "size": [curve(initial.startSize), curve(initial.startSizeY), curve(initial.startSizeZ)], "size3D": bool(initial.size3D),
        "rotation": [curve(initial.startRotationX), curve(initial.startRotationY), curve(initial.startRotation)], "rotation3D": bool(initial.rotation3D), "color": gradient(initial.startColor),
        "emission": {"enabled": bool(emission.enabled), "rate": curve(emission.rateOverTime), "bursts": bursts},
        "shape": {"enabled": bool(shape.enabled), "type": int(shape.type), "radius": float(shape.radius.value), "position": vec(shape.m_Position, "xyz"), "rotation": vec(shape.m_Rotation, "xyz"), "scale": vec(shape.m_Scale, "xyz")},
        "sizeOverLifetime": {"enabled": bool(value.SizeModule.enabled), "x": curve(value.SizeModule.curve), "y": curve(value.SizeModule.y), "z": curve(value.SizeModule.z), "separate": bool(value.SizeModule.separateAxes)},
        "rotationOverLifetime": {"enabled": bool(value.RotationModule.enabled), "x": curve(value.RotationModule.x), "y": curve(value.RotationModule.y), "z": curve(value.RotationModule.curve), "separate": bool(value.RotationModule.separateAxes)},
        "colorOverLifetime": {"enabled": bool(value.ColorModule.enabled), "gradient": gradient(value.ColorModule.gradient)},
        "custom": {"enabled": bool(custom.enabled), "vectors": [[curve(getattr(custom, f"vector{stream}_{axis}")) for axis in range(4)] for stream in range(2)]},
    }

def convert(name):
    env = UnityPy.load(str(SOURCE / f"{name}.ab"))
    objects = {obj.path_id: obj for obj in env.objects}
    assets = {}
    for obj in env.objects:
        if obj.type.name in {"Texture2D", "Mesh"}:
            assets[str(obj.path_id)] = obj.read().m_Name
    materials = {str(obj.path_id): material(obj.read()) for obj in env.objects if obj.type.name == "Material"}
    game_objects = {obj.path_id: obj.read() for obj in env.objects if obj.type.name == "GameObject"}
    transforms = {}
    for obj in env.objects:
        if obj.type.name != "Transform": continue
        value = obj.read()
        transforms[str(obj.path_id)] = {"position": vec(value.m_LocalPosition, "xyz"), "rotation": vec(value.m_LocalRotation, "xyzw"), "scale": vec(value.m_LocalScale, "xyz"), "father": ptr(value.m_Father)}
    systems = {ptr(obj.read().m_GameObject): system(obj.read()) for obj in env.objects if obj.type.name == "ParticleSystem"}
    result = []
    for obj in env.objects:
        if obj.type.name != "ParticleSystemRenderer": continue
        value = obj.read()
        if not value.m_Enabled: continue
        game_id = ptr(value.m_GameObject)
        game = game_objects[game_id]
        transform_id = next((ptr(component.component) for component in game.m_Component if objects.get(ptr(component.component)) and objects[ptr(component.component)].type.name == "Transform"), 0)
        result.append({"name": game.m_Name, "transform": transform_id, "system": systems[game_id], "renderer": {"mode": int(value.m_RenderMode), "alignment": int(value.m_RenderAlignment), "order": int(value.m_SortingOrder), "fudge": float(value.m_SortingFudge), "material": ptr(value.m_Materials[0]) if value.m_Materials else 0, "mesh": ptr(value.m_Mesh)}})
    return {"systems": result, "transforms": transforms, "materials": materials, "assets": assets}

names = sorted(path.stem for path in SOURCE.glob("stt_loop_*.ab"))
OUTPUT.write_text(json.dumps({name: convert(name) for name in names}, separators=(",", ":")) + "\n")
print(f"wrote {len(names)} prefabs to {OUTPUT}")
