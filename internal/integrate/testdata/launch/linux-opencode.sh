#!/bin/sh
{
printf '\033]0;%s\007' 'OpenCode (EZLM)'
export OPENCODE_DISABLE_AUTOUPDATE='1'
cd '/home/sam/EZLM Workspace' || { printf '%s %s\n' "1" "$(date +%s)" > '/home/sam/.ezlm/launch/opencode.exit'; exit 1; }
'/home/sam/.ezlm/tools/opencode/2.2.5/opencode' --standalone --model 'ezlm/qwen3.5-4b'
code=$?
printf '%s %s\n' "$code" "$(date +%s)" > '/home/sam/.ezlm/launch/opencode.exit'
if [ "$code" -ne 0 ]; then echo; printf '%s\n' "OpenCode stopped (exit $code). EZLM shows what went wrong."; echo "Press Enter to close this window."; read ezlm_key; fi
exit $code
}
