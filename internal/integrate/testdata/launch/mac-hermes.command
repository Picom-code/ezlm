#!/bin/zsh
{
printf '\033]0;%s\007' 'Hermes (EZLM)'
cd '/Users/sam/EZLM Workspace' || { print -r -- "1 $(date +%s)" > '/Users/sam/.ezlm/launch/hermes.exit'; exit 1; }
'/Users/sam/.local/bin/hermes' chat --provider 'custom:ezlm' -m 'qwen3.5-4b' 'it'\''s "$HOME"'
code=$?
print -r -- "$code $(date +%s)" > '/Users/sam/.ezlm/launch/hermes.exit'
if (( code != 0 )); then print; print -r -- "Hermes stopped (exit $code). EZLM shows what went wrong."; print "Press any key to close this window."; read -sk1; fi
exit $code
}
