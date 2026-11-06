# App logo sources

The app logos EZLM shows next to the names of the apps it connects to. Each PNG is the official
file below, unmodified (no tint, crop or recolour), rasterised once to a 512x512 RGBA PNG with a
transparent background by headless Chrome (the page showed only the SVG at 512x512;
`page.screenshot({omitBackground: true})`). Downloaded 2026-11-05.

| PNG | Source SVG (in this folder) | Upstream | sha256 of the SVG |
|---|---|---|---|
| app-opencode.png | opencode-logo-dark-square.svg | https://raw.githubusercontent.com/anomalyco/opencode/dev/packages/console/app/src/asset/brand/opencode-logo-dark-square.svg (anomalyco/opencode@2658ec02c848; the square logo for dark backgrounds from https://opencode.ai/brand) | d6a0e3b8a295f413543f41cb73957e670351b5cb088c8d9dbd186b9e9d633cca |
| app-hermes.png | icon-master-dark.svg | https://raw.githubusercontent.com/NousResearch/hermes-agent/main/assets/icon-master-dark.svg (NousResearch/hermes-agent@5d72e9382b23; the Hermes Agent app icon, dark variant) | e1517a2d5b81108f74e938997c442eaa0562af73b7b6beecb3363f3062b8ef36 |
| app-dsh.png | dsh-favicon-dark.svg | https://raw.githubusercontent.com/deepseek-ai/deepseek-harness/master/apps/web/public/favicon-dark.svg (deepseek-ai/deepseek-harness@fcc0ad462172; the harness's own whale for dark backgrounds) | dde7db781f41e7aeb4709ff5485af2bdcf7f4b4ea81b4500cfd5df44efdbd817 |

Owners and licences: OpenCode is Anomaly Innovations, Inc. (repository MIT); Hermes Agent is Nous
Research, Inc. (repository MIT); DeepSeek Harness is DeepSeek (repository MIT). The MIT licences
cover the code, not the trademarks. The logos are used only to identify the app EZLM connects to,
next to its plain name, and never in EZLM's own branding, installer, site hero or app icon.
DeepSeek's BRAND_GUIDELINES.md allows "compatible with DeepSeek Harness" and forbids implying
endorsement. Nous Research publishes no logo policy.

To remove a logo, delete its PNG: the apps fall back to the text wordmark with no code change.
