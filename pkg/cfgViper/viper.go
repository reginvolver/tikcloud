// Package viper
// @Description: 使用Viper进行配置管理
package cfgViper

import (
	"fmt"
	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// Config is the configuration for the viper package.
type Config struct {
	Viper *viper.Viper
}

var (
	configVar      string
	isRemoteConfig bool
)

// init
// @Description: Viper读取配置初始化
func init() {
	pflag.StringVar(&configVar, "config", "", "Config file path")
	pflag.BoolVar(&isRemoteConfig, "isRemoteConfig", false, "Whether to choose remote config")
}

// ConfigInit
// @Description: 初始化配置
// @param envPrefix 前缀
// @param cfgName 配置文件名
// @return Config
func ConfigInit(envPrefix string, cfgName string) Config {
	pflag.Parse()

	v := viper.New()
	config := Config{Viper: v}
	viper := config.Viper
	// 绑定命令行参数
	viper.BindPFlags(pflag.CommandLine)

	// 从环境变量中自动绑定
	viper.AutomaticEnv()

	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if configVar != "" {
		/*
			如果设置了--config参数，尝试从这里解析
			它可能是一个Remote Config，来自etcd或consul
			也可能是一个本地文件
		*/
		u, err := url.Parse(configVar)
		if err != nil {
			klog.Fatalf("error parsing: '%s'", configVar)
		}

		if u.Scheme != "" {
			// url.Scheme不为空，说明是一个远程配置
			config.SetRemoteConfig(u)
			isRemoteConfig = true
		} else {
			// 本地配置
			viper.SetConfigFile(configVar)
		}
	} else {
		// 从默认路径中查找配置文件
		viper.SetConfigName(cfgName) // name of config file (without extension)
		viper.AddConfigPath("/home/tikcloud/config")
		viper.AddConfigPath("$HOME/.tikcloud/")
		viper.AddConfigPath("./config")
		viper.AddConfigPath("../../config")
		viper.AddConfigPath("../../../config")
	}

	if isRemoteConfig {
		if err := viper.ReadRemoteConfig(); err != nil {
			klog.Fatalf("error reading config: %s", err)
		}
		klog.Infof("Using Remote Config: '%s'", configVar)

		viper.WatchRemoteConfig()

		// 另启动一个协程来监测远程配置文件
		go config.WatchRemoteConf()

	} else {
		if err := viper.ReadInConfig(); err != nil {
			klog.Fatalf("error reading config: %s", err)
		}
		klog.Infof("Using configuration file '%s'", viper.ConfigFileUsed())
		viper.WatchConfig()
		viper.OnConfigChange(func(e fsnotify.Event) {
			klog.Info("Config file changed:", e.Name)
		})

	}

	return config
}

func (v *Config) WatchRemoteConf() {
	for {
		time.Sleep(time.Second * 5)

		err := v.Viper.WatchRemoteConfig()
		if err != nil {
			klog.Errorf("unable to read remote config: %v", err)
			continue
		}

		klog.Info("Watching Remote Config")
	}
}

func (v *Config) SetRemoteConfig(u *url.URL) {
	/*
		使用ETCD注册中心的配置
		etcd:
		  url格式为： etcd+http://127.0.0.1:2380/path/to/key.yaml
		  其中：provider=etcd, endpoint=http://127.0.0.1:2380, path=/path/to/key.yaml
	*/

	var provider string
	var endpoint string
	var path string

	schemes := strings.SplitN(u.Scheme, "+", 2)
	if len(schemes) < 1 {
		klog.Fatalf("invalid config scheme '%s'", u.Scheme)
	}

	provider = schemes[0]
	switch provider {

	case "etcd":
		if len(schemes) < 2 {
			klog.Fatalf("invalid config scheme '%s'", u.Scheme)
		}
		protocol := schemes[1]
		endpoint = fmt.Sprintf("%s://%s", protocol, u.Host)
		path = u.Path // u.Path = /path/to/key.yaml
	default:
		klog.Fatalf("unsupported provider '%s'", provider)
	}

	//  配置文件的后缀
	ext := filepath.Ext(path)
	if ext == "" {
		klog.Fatalf("using remote config, without specifiing file extension")
	}
	// .yaml ==> yaml
	configType := ext[1:]

	klog.Infof("Using Remote Config Provider: '%s', Endpoint: '%s', Path: '%s', ConfigType: '%s'", provider, endpoint, path, configType)
	if err := v.Viper.AddRemoteProvider(provider, endpoint, path); err != nil {
		klog.Fatalf("error adding remote provider %s", err)
	}

	v.Viper.SetConfigType(configType)

}
