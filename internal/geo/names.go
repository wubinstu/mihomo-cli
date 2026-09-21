package geo

import "github.com/wubinstu/mihomo-cli/internal/i18n"

// regionZH 常见国家/地区中文名 (ISO -> 中文), 未收录回退 ISO 码
var regionZH = map[string]string{
	"US": "美国", "HK": "香港", "TW": "台湾", "JP": "日本", "SG": "新加坡",
	"KR": "韩国", "DE": "德国", "GB": "英国", "FR": "法国", "CA": "加拿大",
	"AU": "澳大利亚", "NL": "荷兰", "RU": "俄罗斯", "IN": "印度", "BR": "巴西",
	"IT": "意大利", "ES": "西班牙", "CH": "瑞士", "SE": "瑞典", "NO": "挪威",
	"FI": "芬兰", "DK": "丹麦", "PL": "波兰", "CZ": "捷克", "AT": "奥地利",
	"BE": "比利时", "IE": "爱尔兰", "PT": "葡萄牙", "GR": "希腊", "TR": "土耳其",
	"AE": "阿联酋", "SA": "沙特", "IL": "以色列", "TH": "泰国", "VN": "越南",
	"MY": "马来西亚", "ID": "印度尼西亚", "PH": "菲律宾", "NZ": "新西兰",
	"MX": "墨西哥", "AR": "阿根廷", "CL": "智利", "ZA": "南非", "EG": "埃及",
	"UA": "乌克兰", "RO": "罗马尼亚", "BG": "保加利亚", "HU": "匈牙利",
	"LU": "卢森堡", "MO": "澳门", "CN": "中国",
}

var regionEN = map[string]string{
	"US": "United States", "HK": "Hong Kong", "TW": "Taiwan", "JP": "Japan",
	"SG": "Singapore", "KR": "South Korea", "DE": "Germany", "GB": "United Kingdom",
	"FR": "France", "CA": "Canada", "AU": "Australia", "NL": "Netherlands",
	"RU": "Russia", "IN": "India", "BR": "Brazil", "CN": "China",
}

// Name 将 ISO 码转为当前语言的地区名
func Name(iso string) string {
	if iso == "" {
		return ""
	}
	if i18n.Lang() == "zh" {
		if n, ok := regionZH[iso]; ok {
			return n
		}
	} else if n, ok := regionEN[iso]; ok {
		return n
	}
	return iso
}
