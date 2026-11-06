#!/bin/zsh
{
printf '\033]0;%s\007' 'OpenCode (EZLM)'
cd '/Users/o'\''brien/EZLM Workspace' || { print -r -- "1 $(date +%s)" > '/Users/o'\''brien/.ezlm/launch/opencode.exit'; exit 1; }
'/Users/o'\''brien/.ezlm/tools/opencode/2.2.5/opencode' --standalone --model 'ezlm/qwen3.5-4b'
code=$?
print -r -- "$code $(date +%s)" > '/Users/o'\''brien/.ezlm/launch/opencode.exit'
if (( code != 0 )); then print; print -r -- "OpenCode stopped (exit $code). EZLM shows what went wrong."; print "Press any key to close this window."; read -sk1; fi
exit $code
}
