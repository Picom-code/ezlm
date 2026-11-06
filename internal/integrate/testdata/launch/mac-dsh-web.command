#!/bin/zsh
{
printf '\033]0;%s\007' 'DeepSeek Harness (EZLM)'
export DSH_TELEMETRY_MODE='DISABLED'
export PATH='/Users/sam/.ezlm/tools/node/24.11.1/bin:'"${PATH}"
cd '/Users/sam/EZLM Workspace' || { print -r -- "1 $(date +%s)" > '/Users/sam/.ezlm/launch/dsh.exit'; exit 1; }
'/Users/sam/.ezlm/tools/dsh/0.2.1-rc.5/node_modules/.bin/dsh' web --port 3081
code=$?
print -r -- "$code $(date +%s)" > '/Users/sam/.ezlm/launch/dsh.exit'
if (( code != 0 )); then print; print -r -- "DeepSeek Harness stopped (exit $code). EZLM shows what went wrong."; print "Press any key to close this window."; read -sk1; fi
exit $code
}
