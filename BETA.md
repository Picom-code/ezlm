# EZLM beta: how to try it

EZLM sets up your own AI model on your computer and gives you a web address (URL) and key that apps can use. Nothing you type is sent anywhere else.

## Install (Windows 10 or 11)

1. Download **EZLM-Setup.exe** and open it. Already have an older EZLM? Run it anyway: it updates EZLM and keeps your models.
2. Windows may say **"Windows protected your PC"**. That's because this beta isn't code-signed yet. Click **More info**, then **Run anyway**.
3. Click through the installer. It installs just for you (no admin password) and opens EZLM at the end.

Works on regular PCs and on ARM laptops (Snapdragon, NVIDIA RTX Spark). A graphics card makes it much faster (NVIDIA, AMD Radeon RX or Intel Arc), but it also runs without one.

## Install (Mac with Apple Silicon, M1 or later)

1. Open **EZLM.dmg** and drag **EZLM** into **Applications**.
2. Open EZLM from Applications. Because this beta isn't notarized by Apple yet, macOS says it can't verify the app. Click **Done**, then open **System Settings > Privacy & Security**, scroll down, and click **Open Anyway** next to the EZLM message. Confirm with your password or Touch ID.
3. From then on EZLM opens normally. The first time you use **Free up memory**, macOS asks whether EZLM may control your other apps: allow it so EZLM can ask them to quit.

## What happens

- **Scan.** EZLM looks at your processor, memory, graphics card and disk.
- **Choose.** It picks the smartest model that runs well on your computer. Balanced / Smartest / Fastest change the trade-off.
- **Install.** It downloads the AI engine and the model (often 2–60 GB, so plan for a while on slow internet). You can cancel any time; it picks up where it stopped. **Settings** lets you choose the drive or folder models are saved to.
- **Ready.** You get a URL and key. **Chat with it** opens EZLM's own chat with your model. **Open OpenCode** / **Open Hermes** start those apps in a terminal, already using your model; **Connect** wires them up without opening them.

EZLM starts your model when you sign in to Windows (you can turn that off under Task Manager > Startup apps, or press **Stop server**). **Free up memory** (Model mode) walks you through closing apps you're not using, one at a time, so your AI gets more room. Nothing closes without your OK.

## Chat

**Chat** (in the list on the left) talks to your model right inside EZLM. Answers stream in with formatting, and thinking models show their thinking, then fold it away.

- **It remembers you.** Tell it about yourself (your name, your work, what you're building) and later chats know it. Type `/remember` and a fact to add one yourself, or `/forget` and a word to remove what mentions it. **✦ memory** shows everything it keeps: edit it, add your own instructions ("keep answers short"), or turn memory off.
- **History** reopens earlier chats. **+ file** (or drag a file in) attaches a text file. **Retry** and **Listen** are under each answer, and **Model** at the top switches models.
- Chats and memory stay on your computer: `%LOCALAPPDATA%\EZLM` on Windows, `~/Library/Application Support/EZLM` on a Mac.

## Updates

**Settings > Updates** shows when a new version is out. One click downloads it, checks it, installs it and reopens EZLM; your model keeps running.

## What to try, and what to tell us

- Set up a model, ask it something in **Chat**, tell it your name, then start a **New chat** and ask what your name is.
- Leave it running for a day and use it from another app (OpenCode, Hermes, or anything with the URL and key).
- **AMD Radeon or Intel Arc graphics card?** Please tell us, even if all goes well: the **Graphics** and **Engine** lines on the **System** page (the engine should say Vulkan), the model EZLM picked, and its speed on **Overview**. If a gold **Running on the processor** note appears there, send a report.
- **Windows in another language, an older or small-screen laptop, or no graphics card?** A quick "it worked" or a report helps just as much: those are the PCs we've tested least.

## If something goes wrong

Press **Report a problem** (bottom left, on any error, or under **Logs**), say what happened, and send it. Reports are posted on EZLM's public GitHub page, so anyone can read them. A report can include your hardware summary and recent logs; your API keys, your files and what you typed to the model are never included. If it opens on GitHub instead of sending directly, press **Submit new issue** there. **Copy details for support** copies the same details to paste anywhere.

## Uninstall

**Windows:** Settings > Apps > Installed apps > **EZLM** > Uninstall. It asks whether to also delete the downloaded models. Choose **No** to keep them for a later reinstall, or **Yes** to free the disk space.

**Mac:** in EZLM, open **System** and press **Remove models and settings** (this stops the model server and removes it from startup), then drag EZLM from Applications to the Trash.

## Known limits of this beta

- Not code-signed yet, so you'll see the Windows SmartScreen warning or the macOS "can't verify" message described above. On a Windows 11 PC with **Smart App Control** turned on, Windows blocks unsigned apps outright, with no Run anyway button, so EZLM can't be installed there until it's signed.
- First model downloads are large; a fast connection helps.
- Hermes needs a model with a 64K context; EZLM tells you when a model can't give it that without slowing down.
