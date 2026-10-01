# Account player-frame overrides

`pjsk_render.player_frame_overrides` in the Cloud YAML file lets an operator replace
an account's equipped player frame with six local sprites. Entries match the
resolved server (`jp`, `cn`, `tw`, `en`, `kr`) and actual game UID, not the QQ user
or request-supplied profile UID. The override applies to live, snapshot, modular
profiles and profile cards.
Every builder resolves frames through `playerframe.ResolveAccount`, so the
suite-backed info panel on card, deck, education, event, inventory, MySekai and
`/信息面板` renders shows the same frame as `/profile`. Other accounts keep their game-equipped frames.

```yaml
pjsk_render:
  player_frame_overrides:
    - server: cn
      user_id: "1234567890123456789"
      horizontal:
        base: static_images/custom_frames/my_frame/horizontal/frame_base.png
        centertop: static_images/custom_frames/my_frame/horizontal/frame_centertop.png
        lefttop: static_images/custom_frames/my_frame/horizontal/frame_lefttop.png
        righttop: static_images/custom_frames/another_frame/horizontal/frame_righttop.png
        leftbottom: static_images/custom_frames/my_frame/horizontal/frame_leftbottom.png
        rightbottom: static_images/custom_frames/my_frame/horizontal/frame_rightbottom.png
      vertical: # optional
        base: static_images/custom_frames/my_frame/vertical/frame_base.png
        centertop: static_images/custom_frames/my_frame/vertical/frame_centertop.png
        lefttop: static_images/custom_frames/my_frame/vertical/frame_lefttop.png
        righttop: static_images/custom_frames/my_frame/vertical/frame_righttop.png
        leftbottom: static_images/custom_frames/my_frame/vertical/frame_leftbottom.png
        rightbottom: static_images/custom_frames/my_frame/vertical/frame_rightbottom.png
```

Each part is independent: it need not share a folder or follow the game's bundle
naming. Paths must be clean, relative `static_images/...` paths under Drawing's
configured asset directory. URLs, absolute paths and traversal paths are rejected.
All six fields are required; an intentionally blank slot uses a transparent PNG.
Duplicate account entries and invalid server/UID values fail configuration loading.
For the finale horizontal format the base is 60×60, the four corner sprites are
536×82 and the top-centre sprite is 62×36. Drawing retains its existing native
slice/anchor layout and unreadable-asset fallback.

There is no environment override, database field, API setter or bot command for
these settings. Cloud loads the file at startup: restart Cloud after edits.
Remove an entry (or use an empty list) to restore the game's equipped frame.
This is a game-account setting and applies to every binding of the same server/UID.

## Cells

The game draws every frame in two cells, and so does Drawing:

- `horizontal` — the list row (friend list, rankings; 1542×146 reference). The info panel
  atop other pages (profile card) and `/信息面板` use it. **Required.**
- `vertical` — the player cell (multi-live room, frame-setting preview; 340×748 reference).
  The `/profile` info panel uses it. **Optional**: when an entry has no `vertical` block,
  `/profile` falls back to the horizontal sprites, so a custom frame never disappears.

The vertical block follows the same rules as the horizontal one (six independent, clean
`static_images/...` paths, all required once the block is present). For the game's vertical
format the base is 132×132, the top corner strips are 130×352, the bottom-left strip 226×352,
the bottom-right strip 72×352 and the top-centre crown 108×90; Drawing pins them to the cell
exactly as the client's `PlayerFrameCell` prefab does.

Deploy Drawing with explicit `frame_paths.horizontal` support (and `frame_paths.vertical`
support before adding vertical blocks) before enabling this configuration in Cloud.
