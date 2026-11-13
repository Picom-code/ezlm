<p align="center">
  <a href="https://picom-code.github.io/ezlm/"><img src=".github/hero.png" alt="EZLM: your own AI, running on your computer" width="100%"></a>
</p>

<p align="center">
  <a href="https://github.com/Picom-code/ezlm/releases/latest"><img src="https://img.shields.io/github/v/release/Picom-code/ezlm?include_prereleases&label=beta&style=flat-square&labelColor=000000&color=b9a8ff"></a>
  <img src="https://img.shields.io/badge/Windows-10%20%2F%2011%20·%20x64%20%2B%20ARM-bbbbbb?style=flat-square&labelColor=000000">
  <img src="https://img.shields.io/badge/macOS-14%2B%20·%20Apple%20Silicon-bbbbbb?style=flat-square&labelColor=000000">
  <img src="https://img.shields.io/badge/account-none-bbbbbb?style=flat-square&labelColor=000000">
</p>

<p align="center">
  <a href="https://github.com/Picom-code/ezlm/releases/latest/download/EZLM-Setup.exe"><b>Download for Windows</b></a>
  &nbsp;·&nbsp;
  <a href="https://picom-code.github.io/ezlm/#download"><b>Download for Mac</b></a>
  &nbsp;·&nbsp;
  <a href="https://picom-code.github.io/ezlm/">picom-code.github.io/ezlm</a>
</p>

# EZLM

Your own AI model, running on your own computer. EZLM checks your machine, picks the smartest model it can
run well, sets it up, and hands you a private URL and API key. No account, no cloud, no command line.

## How it works

1. **Scan.** Checks your processor, memory, graphics card and disk, and what's already using them.
2. **Choose.** Picks the smartest model that runs well here. Pick Balanced, Smartest or Fastest if you want a say.
3. **Install.** Downloads the AI engine and the model. Cancel any time; it picks up where it stopped.
4. **Ready.** Chat in EZLM right away, or use your URL and key in any app that speaks the OpenAI API.

## What you get

| Feature | What it does |
|---|---|
| **Chat built in** | Talk to your model inside EZLM. It remembers what you tell it, and you can see and edit everything it keeps. |
| **Switch models any time** | Your address, keys and connected apps stay the same. The current model keeps answering while a new one downloads. |
| **Model mode** | Frees memory by asking, one app at a time, whether it can close the ones you're not using. Nothing closes without your OK. |
| **Keys you control** | One key per device. Rotate or revoke one without breaking the others. |
| **Plugs into your tools** | One click connects OpenCode, Hermes or DeepSeek Harness, and checks that each one really works. |
| **Stays on your computer** | The model runs on your hardware. Nothing you type is sent anywhere else. |

## Use it from anything

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8080/v1",
    api_key="ezlm-...",  # shown in EZLM
)
```

## What your computer needs

| | Windows | Mac |
|---|---|---|
| **System** | Windows 10 (2004+) or 11, x64 and ARM | macOS 14+, Apple Silicon |
| **Graphics** | NVIDIA is fastest; AMD and Intel run through Vulkan; no GPU works too, just slower | Uses the Mac's built-in graphics |
| **Memory** | 16 GB or more is a good start; more memory means a smarter model | Same; the model shares the Mac's memory |
| **Disk** | Under 1 GB to about 72 GB, depending on the model | Same |

## Installing the beta

The beta isn't code-signed yet, so both systems ask you to confirm the first time.

- **Windows:** open `EZLM-Setup.exe`. If you see *Windows protected your PC*, click **More info**, then **Run anyway**.
  It installs just for you, with no administrator password, and updates itself from then on.
  (With Smart App Control on, Windows 11 blocks unsigned apps outright.)
- **Mac:** drag EZLM into Applications and open it. Close the "can't verify" message, then go to
  **System Settings > Privacy & Security** and click **Open Anyway**.

Step-by-step with pictures: [picom-code.github.io/ezlm/#install](https://picom-code.github.io/ezlm/#install).
Release notes: [releases](https://github.com/Picom-code/ezlm/releases).

## Something not right?

In EZLM, press **Report a problem** (bottom left), or open an [issue](https://github.com/Picom-code/ezlm/issues).
Your keys and files are never included.
