# stdiag demo — 3 scenes showing core value
Output demo.gif
Set Shell zsh
Set FontSize 15
Set Width 900
Set Height 700
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms

Require stdiag

# Scene 1: All layers pass
Type "# All layers OK"
Enter
Sleep 300ms
Type "stdiag https://example.com"
Sleep 200ms
Enter
Sleep 4s

# Scene 2: DNS failure — stop at first failure
Type ""
Enter
Type "# DNS fails → everything else skipped"
Enter
Sleep 300ms
Type "stdiag https://nonexistent.example.com"
Sleep 200ms
Enter
Sleep 4s

# Scene 3: JSON output with error classification
Type ""
Enter
Type "# Structured JSON for AI agents"
Enter
Sleep 300ms
Type "stdiag --json-pretty https://self-signed.badssl.com 2>/dev/null | head -30"
Sleep 200ms
Enter
Sleep 5s

Sleep 2s
