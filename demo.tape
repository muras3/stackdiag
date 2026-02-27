# stdiag demo — layer-by-layer network diagnostics
Output demo.gif
Set Shell zsh
Set FontSize 16
Set Width 900
Set Height 400
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms

Require stdiag

# Healthy endpoint
Type "stdiag https://example.com"
Sleep 300ms
Enter
Sleep 4s

# Broken TLS — stops at the layer that broke
Type "stdiag https://expired.badssl.com"
Sleep 300ms
Enter
Sleep 5s
