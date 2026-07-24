package bridge

import "github.com/pkg/browser"

const footerAuthorHomeURL = "https://github.com/Cao-Junqi/cursor-byok"

var footerAuthorInfo = FooterAuthorInfo{
	ButtonText:        "作者 Cao-Junqi",
	DialogTitle:       "关于",
	DialogContent:     "本项目由 Cao-Junqi 维护，基于 leookun/cursor-byok 修复和改进。\n欢迎访问主页：https://github.com/Cao-Junqi/cursor-byok",
	DialogConfirmText: "访问主页",
	DialogCancelText:  "关闭",
}

// FooterAuthorInfo 定义首页底部作者入口的展示信息。
type FooterAuthorInfo struct {
	ButtonText        string `json:"buttonText"`
	DialogTitle       string `json:"dialogTitle"`
	DialogContent     string `json:"dialogContent"`
	DialogConfirmText string `json:"dialogConfirmText"`
	DialogCancelText  string `json:"dialogCancelText"`
}

// GetFooterAuthorInfo 返回首页底部作者入口的展示信息。
func (s *WindowService) GetFooterAuthorInfo() FooterAuthorInfo {
	return footerAuthorInfo
}

// OpenFooterAuthorHome 打开作者主页。
func (s *WindowService) OpenFooterAuthorHome() error {
	return browser.OpenURL(footerAuthorHomeURL)
}
