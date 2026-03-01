Output demo-readme.gif
Set Shell "bash"
Set FontSize 20
Set Width 1000
Set Height 650
Require echo


Type "rv init --template 4wd-problem"
Enter
Sleep 1s


Type "cat robot.yaml"
Enter
Sleep 2s


Type "rv check robot.yaml"
Enter
Sleep 3s


Type "rv init --template 4wd-clean --out robot.yaml --force"
Enter
Sleep 1s

Type "cat robot.yaml"
Enter
Sleep 2s

Type "rv check robot.yaml"
Enter
Sleep 3s