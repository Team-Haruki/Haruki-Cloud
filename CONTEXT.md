# Haruki Cloud

Haruki Cloud interprets bot commands and resolves Project SEKAI data into user-facing results.

## 3D Costume Language

**3D Character ID**:
The 1–31 `character3dId` that identifies one default 3D role, including each SEKAI-specific Miku as a distinct role.
_Avoid_: character ID, unit

**Outfit ID**:
A Haruki-owned identifier for one logical outfit across compatible characters and color variants.
_Avoid_: body costume ID, costume3dId, costume3dGroupId

**Accessory ID**:
A Haruki-owned identifier for one logical accessory across its allowed 3D roles and color variants. Character-exclusive availability remains part of the accessory definition.
_Avoid_: head ID, head_optional ID, costume3dId

**Component Color ID**:
The color variant local to one outfit or accessory; color 1 is the original. A color always modifies the component immediately before it.
_Avoid_: global color
