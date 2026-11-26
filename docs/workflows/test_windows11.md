# 测试工作流
可使用技能：
lz-auto 自动化技能

读一下这个避坑指南:docs/测试避坑指南.md

如果在测试中解决了某些卡住的问题，也回写到避坑指南，一条规则就行。

先读取前一天到现在的变动功能，进行测试，测试后保留步骤、日志、图片证据

切换到main先执行 git pull 拉取最新代码

保存到`docs/test/<yyyy-MM-dd>` 当前日期目录， 图片也放这个下，最后生成一个 windows11_report.html文件

然后提交推送，注释写 test(windows11): <生成50字以内的注释>

测试目标设备：Windows11 ， 这是一台Windows11 x86设备，下载（https://github.com/liuzhuogood/Farm/releases）最新发布包进行测试，替换安装（桌面/Farm.exe）

测试不通过使用 lz-workflow issues.md 提交问题
