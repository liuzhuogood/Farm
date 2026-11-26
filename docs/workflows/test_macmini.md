# 测试工作流
可使用技能：
lz-auto 自动化技能

先读取上次测试的日期：`docs/test/<yyyy-MM-dd>` ，从git提交上获取上次日期到现在的变动功能，进行测试，测试后保留步骤、日志、图片证据

保存到`docs/test/<yyyy-MM-dd>` 当前日期目录， 图片也放这个下，最后生成一个 report.html文件

然后提交推送，注释写 test(macmini): <生成50字以内的注释>

测试目标设备： 10.0.0.11， 这是一台macmini设备，下载（https://github.com/liuzhuogood/Farm/releases）最新发布包进行测试，替换安装（/Applications/Farm.app）

