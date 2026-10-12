// Package config is the authoritative, fail-closed parser for both profile types.
package config

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Profile struct {
	Protocol string   `json:"protocol"`
	Text     string   `json:"text"`
	Sources  []string `json:"sources"` // Empty: tunnel only. Explicit CIDRs: source policy routing.
}
type Parsed struct {
	Interface map[string]string
	Peers     []map[string]string
	Addresses []netip.Prefix
	Routes    []netip.Prefix
	Sources   []netip.Prefix
	MTU       int
	Warnings  []string
}

var names = map[string]string{
	"PrivateKey": "private_key", "ListenPort": "listen_port", "Jc": "jc", "Jmin": "jmin", "Jmax": "jmax",
	"S1": "s1", "S2": "s2", "S3": "s3", "S4": "s4", "H1": "h1", "H2": "h2", "H3": "h3", "H4": "h4",
	"I1": "i1", "I2": "i2", "I3": "i3", "I4": "i4", "I5": "i5", "HeaderProtectionKey": "header_protection_key",
	"ContentPaddingAddition": "content_padding_addition", "RekeyAfterTime": "rekey_after_time", "RekeyTimeout": "rekey_timeout",
	"RejectAfterTime": "reject_after_time", "KeepaliveTimeout": "keepalive_timeout", "MaxHandshakeAttempts": "max_handshake_attempts",
	"RandomTrailers": "random_trailers", "DisableCookies": "disable_cookies",
}
var peerNames = map[string]string{"PublicKey": "public_key", "PresharedKey": "preshared_key", "Endpoint": "endpoint", "PersistentKeepalive": "persistent_keepalive_interval", "AllowedIPs": "allowed_ip"}
var integer = regexp.MustCompile(`^[0-9]+$`)
var signature = regexp.MustCompile(`<(?:b 0x[0-9a-fA-F]+|r [0-9]+|rd [0-9]+|rc [0-9]+|t)>`)
var hostname = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

func number(s string, max uint64) (uint64, error) {
	if !integer.MatchString(s) {
		return 0, errors.New("必须是十进制非负整数")
	}
	v, e := strconv.ParseUint(s, 10, 32)
	if e != nil || v > max {
		return 0, fmt.Errorf("超出 0–%d", max)
	}
	return v, nil
}
func span(s string, max uint64, nonzero bool) (uint64, uint64, error) {
	a := strings.Split(s, "-")
	if len(a) > 2 {
		return 0, 0, errors.New("区间应为 a-b")
	}
	lo, e := number(a[0], max)
	if e != nil {
		return 0, 0, e
	}
	hi := lo
	if len(a) == 2 {
		hi, e = number(a[1], max)
	}
	if e != nil || hi < lo || (nonzero && lo == 0) {
		return 0, 0, errors.New("区间无效或含不允许的零")
	}
	return lo, hi, nil
}
func key(s string) (string, error) {
	b, e := base64.StdEncoding.Strict().DecodeString(s)
	if e != nil || len(b) != 32 || base64.StdEncoding.EncodeToString(b) != s {
		return "", errors.New("密钥必须为规范 Base64 编码的 32 字节")
	}
	var v byte
	for _, x := range b {
		v |= x
	}
	if v == 0 {
		return "", errors.New("不接受全零密钥")
	}
	return hex.EncodeToString(b), nil
}
func prefixes(s string, canonical bool) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, v := range strings.Split(s, ",") {
		p, e := netip.ParsePrefix(strings.TrimSpace(v))
		if e != nil || p.Addr().Is4In6() || p.Addr().IsMulticast() {
			return nil, errors.New("无效的 IPv4 / IPv6 CIDR")
		}
		if canonical && p != p.Masked() {
			return nil, errors.New("路由 CIDR 含主机位，请使用网络地址")
		}
		out = append(out, p)
	}
	return out, nil
}
func checkSignature(s string) error {
	tags := signature.FindAllString(s, -1)
	if strings.Join(tags, "") != s {
		return errors.New("仅接受连续的 b / r / rc / rd / t 标签")
	}
	n := 0
	for _, t := range tags {
		body := strings.TrimSuffix(strings.TrimPrefix(t, "<"), ">")
		a := strings.Split(body, " ")
		switch a[0] {
		case "b":
			v := strings.TrimPrefix(a[1], "0x")
			if len(v)%2 != 0 {
				return errors.New("十六进制字节必须成对")
			}
			n += len(v) / 2
		case "t":
			n += 4
		default:
			v, e := number(a[1], 1400)
			if e != nil {
				return e
			}
			n += int(v)
		}
	}
	if n < 1 || n > 1400 {
		return errors.New("签名包长度必须为 1–1400 字节（本插件限制）")
	}
	return nil
}
func Parse(p Profile) (*Parsed, error) {
	if p.Protocol != "wireguard" && p.Protocol != "amneziawg" {
		return nil, errors.New("请选择 WireGuard 或 AmneziaWG")
	}
	if len(p.Text) > 65536 || strings.ContainsAny(p.Text, "\x00\u2028\u2029") {
		return nil, errors.New("配置过长或含控制字符")
	}
	c := &Parsed{Interface: map[string]string{}, MTU: 1280, Warnings: []string{}}
	var section map[string]string
	isPeer := false
	seenInterface := false
	for index, line := range strings.Split(strings.ReplaceAll(p.Text, "\r\n", "\n"), "\n") {
		s := strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if s == "" || strings.HasPrefix(s, ";") {
			continue
		}
		if s == "[Interface]" {
			if seenInterface || len(c.Peers) > 0 {
				return nil, errors.New("Interface 必须唯一且位于 Peer 前")
			}
			seenInterface = true
			section = c.Interface
			continue
		}
		if s == "[Peer]" {
			if !seenInterface {
				return nil, errors.New("缺少 Interface")
			}
			isPeer = true
			section = map[string]string{}
			c.Peers = append(c.Peers, section)
			if len(c.Peers) > 32 {
				return nil, errors.New("最多支持 32 个 Peer")
			}
			continue
		}
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 || section == nil {
			return nil, fmt.Errorf("第 %d 行格式无效", index+1)
		}
		k, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if v == "" {
			return nil, fmt.Errorf("第 %d 行值为空", index+1)
		}
		if _, ok := section[k]; ok {
			return nil, fmt.Errorf("第 %d 行字段重复", index+1)
		}
		if isPeer {
			if _, ok := peerNames[k]; !ok {
				return nil, fmt.Errorf("第 %d 行不是支持的 Peer 字段（AdvancedSecurity 不受固定后端支持）", index+1)
			}
		} else {
			_, ok := names[k]
			if !ok && k != "Address" && k != "MTU" && k != "DNS" {
				return nil, fmt.Errorf("第 %d 行包含不支持的字段；不执行配置脚本", index+1)
			}
			if p.Protocol == "wireguard" && ok && k != "PrivateKey" && k != "ListenPort" {
				return nil, fmt.Errorf("第 %d 行包含 AmneziaWG 扩展，请切换协议", index+1)
			}
		}
		section[k] = v
	}
	if !seenInterface || len(c.Peers) == 0 {
		return nil, errors.New("配置需要 Interface 和至少一个 Peer")
	}
	if _, e := key(c.Interface["PrivateKey"]); e != nil {
		return nil, fmt.Errorf("PrivateKey: %w", e)
	}
	var e error
	c.Addresses, e = prefixes(c.Interface["Address"], false)
	if e != nil {
		return nil, fmt.Errorf("Address: %w", e)
	}
	if v, ok := c.Interface["MTU"]; ok {
		n, err := number(v, 1420)
		if err != nil || n < 1280 {
			return nil, errors.New("MTU 支持 1280–1420（双栈安全范围）")
		}
		c.MTU = int(n)
	}
	if v, ok := c.Interface["DNS"]; ok {
		for _, x := range strings.Split(v, ",") {
			if _, e := netip.ParseAddr(strings.TrimSpace(x)); e != nil {
				return nil, errors.New("DNS 仅接受 IP 地址")
			}
		}
		c.Warnings = append(c.Warnings, "DNS 仅保存供参考：本版不改 Android 全局 DNS，也不添加 DNS DNAT。")
	}
	ranges := [][2]uint64{{1, 1}, {2, 2}, {3, 3}, {4, 4}}
	for k, v := range c.Interface {
		var err error
		switch {
		case k == "Address" || k == "DNS" || k == "MTU":
		case k == "PrivateKey" || k == "HeaderProtectionKey":
			_, err = key(v)
		case k == "ListenPort":
			_, err = number(v, 65535)
		case k == "Jc":
			_, err = number(v, 64)
		case k == "Jmin" || k == "Jmax":
			_, err = number(v, 1400)
		case strings.HasPrefix(k, "S") && len(k) == 2:
			_, err = number(v, 1024)
		case strings.HasPrefix(k, "H") && len(k) == 2:
			lo, hi, er := span(v, 4294967295, true)
			err = er
			ranges[int(k[1]-'1')] = [2]uint64{lo, hi}
		case strings.HasPrefix(k, "I") && len(k) == 2:
			err = checkSignature(v)
		case k == "RandomTrailers" || k == "DisableCookies":
			if v != "true" && v != "false" {
				err = errors.New("仅接受 true / false")
			}
		case k == "ContentPaddingAddition":
			_, _, err = span(v, 1024, false)
		default:
			_, _, err = span(v, 86400, true)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
	}
	for i, a := range ranges {
		for j := i + 1; j < 4; j++ {
			b := ranges[j]
			if a[0] <= b[1] && b[0] <= a[1] {
				return nil, errors.New("H1–H4 的有效区间不能重叠（包括未填字段的默认值）")
			}
		}
	}
	jmin, _ := number(c.Interface["Jmin"], 1400)
	jmax, _ := number(c.Interface["Jmax"], 1400)
	if jmin > jmax {
		return nil, errors.New("Jmin 不能大于 Jmax")
	}
	if c.Interface["HeaderProtectionKey"] != "" {
		for _, k := range []string{"S1", "S2", "S3", "S4"} {
			v, _ := number(c.Interface[k], 1024)
			if v < 12 {
				return nil, errors.New("头部保护要求 S1–S4 均至少 12")
			}
		}
	}
	seenKeys := map[string]bool{}
	seenRoutes := map[string]bool{}
	for i, peer := range c.Peers {
		pk, err := key(peer["PublicKey"])
		if err != nil {
			return nil, fmt.Errorf("Peer %d PublicKey 无效", i+1)
		}
		if seenKeys[pk] {
			return nil, errors.New("Peer 公钥重复")
		}
		seenKeys[pk] = true
		if v := peer["PresharedKey"]; v != "" {
			if _, e := key(v); e != nil {
				return nil, fmt.Errorf("Peer %d PresharedKey 无效", i+1)
			}
		}
		if v := peer["Endpoint"]; v != "" {
			host, port, e := net.SplitHostPort(v)
			if e != nil {
				return nil, fmt.Errorf("Peer %d Endpoint 需要 host:port，IPv6 使用方括号", i+1)
			}
			n, e := number(port, 65535)
			if e != nil || n == 0 {
				return nil, errors.New("Endpoint 端口无效")
			}
			if _, e := netip.ParseAddr(host); e != nil {
				if !hostname.MatchString(host) || strings.Contains(host, "..") {
					return nil, errors.New("Endpoint 主机名无效")
				}
				c.Warnings = append(c.Warnings, "域名 Endpoint 启动时使用设备解析器；解析失败会停止，不使用公共 DNS 回退。")
			}
		}
		if v := peer["PersistentKeepalive"]; v != "" {
			max := uint64(86400)
			if p.Protocol == "wireguard" {
				max = 65535
				if strings.Contains(v, "-") {
					return nil, errors.New("WireGuard keepalive 不支持区间")
				}
			}
			if _, _, e := span(v, max, false); e != nil {
				return nil, errors.New("PersistentKeepalive 无效")
			}
		}
		routes, e := prefixes(peer["AllowedIPs"], true)
		if e != nil {
			return nil, fmt.Errorf("Peer %d AllowedIPs: %w", i+1, e)
		}
		for _, r := range routes {
			if seenRoutes[r.String()] {
				return nil, errors.New("Peer 之间不能重复 AllowedIPs")
			}
			seenRoutes[r.String()] = true
			c.Routes = append(c.Routes, r)
		}
	}
	if len(p.Sources) > 16 {
		return nil, errors.New("来源网段最多 16 个")
	}
	seenSources := map[string]bool{}
	for _, s := range p.Sources {
		ps, e := prefixes(s, true)
		if e != nil || len(ps) != 1 {
			return nil, errors.New("路由来源必须是单个网络 CIDR")
		}
		r := ps[0]
		if r.Bits() == 0 || r.Addr().IsUnspecified() || r.Addr().IsLoopback() {
			return nil, errors.New("来源不能是默认、未指定或回环网段")
		}
		if seenSources[r.String()] {
			return nil, errors.New("路由来源重复")
		}
		seenSources[r.String()] = true
		c.Sources = append(c.Sources, r)
	}
	if len(c.Sources) == 0 {
		c.Warnings = append(c.Warnings, "仅创建独立隧道；未启用来源策略路由，不接管设备默认网络。")
	}
	return c, nil
}

// UAPI always configures a fresh device. DNS resolution is supplied by the caller.
func (c *Parsed) UAPI(resolve func(string) (string, error)) (string, error) {
	var b strings.Builder
	keys := make([]string, 0, len(c.Interface))
	for k := range c.Interface {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		n, ok := names[k]
		if !ok {
			continue
		}
		v := c.Interface[k]
		if k == "PrivateKey" || k == "HeaderProtectionKey" {
			v, _ = key(v)
		}
		fmt.Fprintf(&b, "%s=%s\n", n, v)
	}
	b.WriteString("replace_peers=true\n")
	for _, peer := range c.Peers {
		v, _ := key(peer["PublicKey"])
		fmt.Fprintf(&b, "public_key=%s\nreplace_allowed_ips=true\n", v)
		for _, k := range []string{"PresharedKey", "Endpoint", "PersistentKeepalive", "AllowedIPs"} {
			v, ok := peer[k]
			if !ok {
				continue
			}
			if k == "PresharedKey" {
				v, _ = key(v)
			}
			if k == "Endpoint" {
				var e error
				v, e = resolve(v)
				if e != nil {
					return "", errors.New("Endpoint 解析失败；请提供设备可解析的域名或 IP")
				}
			}
			for _, x := range strings.Split(v, ",") {
				fmt.Fprintf(&b, "%s=%s\n", peerNames[k], strings.TrimSpace(x))
			}
		}
	}
	return b.String(), nil
}
