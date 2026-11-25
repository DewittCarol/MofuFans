package controller

import (
	"errors"
	"github.com/gin-gonic/gin"
	"mofufans/internal/database"
	"mofufans/internal/theme"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

type FooterSettings struct {
	ICPNumber    string `json:"footer_icp_number"`
	PoliceNumber string `json:"footer_police_number"`
	PoliceURL    string `json:"footer_police_url"`
	PoliceIcon   string `json:"footer_police_icon"`
}

var filingDigits = regexp.MustCompile(`[0-9]{14}`)

func (f *FooterSettings) validate() error {
	f.ICPNumber = strings.TrimSpace(f.ICPNumber)
	f.PoliceNumber = strings.TrimSpace(f.PoliceNumber)
	f.PoliceURL = strings.TrimSpace(f.PoliceURL)
	f.PoliceIcon = strings.TrimSpace(f.PoliceIcon)
	if utf8.RuneCountInString(f.ICPNumber) > 80 || utf8.RuneCountInString(f.PoliceNumber) > 80 || strings.ContainsAny(f.ICPNumber+f.PoliceNumber, "\r\n\t") {
		return errors.New("备案号请使用不超过 80 字的单行文字")
	}
	if f.PoliceNumber != "" && f.PoliceURL == "" {
		code := filingDigits.FindString(f.PoliceNumber)
		if code == "" {
			return errors.New("请填写公安备案查询链接，或在备案号中包含 14 位数字")
		}
		f.PoliceURL = "https://beian.mps.gov.cn/#/query/webSearch?code=" + code
	}
	if f.PoliceURL != "" {
		u, err := url.Parse(f.PoliceURL)
		if err != nil || len(f.PoliceURL) > 2048 || u.Scheme != "https" || u.User != nil || u.Port() != "" || !(u.Host == "beian.mps.gov.cn" || u.Host == "beian.gov.cn" || u.Host == "www.beian.gov.cn") {
			return errors.New("公安备案查询链接请使用 HTTPS 的官方备案网站地址")
		}
	}
	if !theme.AssetURL(f.PoliceIcon) {
		return errors.New("备案图标请上传图片，或使用 HTTPS 图片链接")
	}
	return nil
}
func (*ConfigController) PutFooter(c *gin.Context) {
	var request FooterSettings
	if !installationBody(c, &request) {
		return
	}
	if err := request.validate(); err != nil {
		Fail(c, 400, err.Error())
		return
	}
	var config database.Config
	if database.DB.First(&config).Error != nil {
		Fail(c, 500, "页脚设置读取失败")
		return
	}
	if database.DB.Model(&config).Updates(map[string]any{"footer_icp_number": request.ICPNumber, "footer_police_number": request.PoliceNumber, "footer_police_url": request.PoliceURL, "footer_police_icon": request.PoliceIcon}).Error != nil {
		Fail(c, 500, "页脚设置保存失败")
		return
	}
	Success(c, request)
}
