Output demo-readme.gif
Set Shell "bash"
Set FontSize 20
Set Width 1000
Set Height 650
Require echo
Set PlaybackSpeed 0.7

Type "cd demos/demo1-esp32-motor/kicad"
Enter

Sleep 1s

Type "rv scan ."
Enter

Sleep 4s

Type "rv init --template 4wd-problem"
Enter

Sleep 2s

Type "rv check robot.yaml"
Enter

Sleep 6s