# Account player-frame overrides

`pjsk_render.player_frame_overrides` in the Cloud YAML file lets an operator replace
an account's equipped player frame with six local sprites. Entries match the
resolved server (`jp`, `cn`, `tw`, `en`, `kr`) and actual game UID, not the QQ user
or request-supplied profile UID. The override applies to live, snapshot, modular
profiles and profile cards. Other accounts keep their game-equipped frames.

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

Deploy Drawing with explicit `frame_paths.horizontal` support before enabling this
configuration in Cloud. These sprites decorate horizontal player cells; a vertical
player-cell override is not supplied by this configuration.
