# shimo-export
石墨文档批量导出

# 前提
1. go运行环境
2. 安装pandoc，且加入了环境变量

# 使用步骤
1. cp config.example.json config.json
2. 修改config.json文件。（主要修改的是导出目录和Cookie）
3. 运行main函数

# 一些提示
1. 本代码默认导出word，然后通过pandoc转换为md文件（直接导出md文件，里面的图片是不可用的，且图片采用的还是石墨文档的云图片，不能掌握在自己手中。） ，md文件的图片会存放在文章目录下，以文章标题为名的文件夹下（一个文件一个目录），md文件里的图片是相对路径。（方便后续迁移）
2. 如果希望转换为pdf文件，需要把代码中的docx改成pdf，然后把pandoc转换的代码注释掉。（不需要转换了）
3. 这个导出接口，一分钟5次限流，代码中当触发频繁了之后，也会自动进行休眠。
4. cookie可能会过期，程序在运行中，不需要停止，只需要修改配置文件中的cookie参数，有一个协程自动更新相关配置。
5. 如果石墨文档中涉及到评论，也会把评论数据保存在文档标题文件夹中的comments.json文件中。（我放到这个文件的目的是未来可以读取这些评论到我自己给自己开发的笔记软件里）

# 评论结构

在石墨文档中评论，首先你需要先选择一段文字，然后可以对这段文字进行评论。那么这段文字就会有一个唯一的id，叫SelectionGuid.

```go
type CommentGroup struct {
    SelectionGuid    string `json:"selectionGuid"`  // 选择的这段文字的唯一标识
    SelectionContent string `json:"selectionContent"`  // 选择的这段文字的文本内容
    Comments         []struct {  // 这段文本下的所有评论
        CommentGuid string `json:"commentGuid"`  // 单个评论id
        Content     string `json:"content"`  // 评论的内容
        Name        string `json:"name"`  // 评论人昵称
        ReplyTo     string `json:"replyTo"`  // 如果此条评论是回复某个评论，这个值为那个评论的评论id，否则为空。
    } `json:"comments"`
}
```
