package main

import (
	bytes1 "bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type Excel struct {
	ExportFile string                 `json:"export_file"`
	Root       string                 `json:"root"`
	Item       string                 `json:"item"`
	Schema     map[string]interface{} `json:"schema"`
}

type FM struct {
	BundlesNeedPreload []string  `json:"bundlesNeedPreload"`
	ConfigsNeedPreload []Configs `json:"configsNeedPreload"`
}

type Configs struct {
	Name        string `json:"name"`
	Url         string `json:"url"`
	LocalUrl    string `json:"localUrl"`
	LocalPrefer bool   `json:"localPrefer"`
}

var fileName = flag.String("p", "schema.json", "schema文件名")
var outFile = flag.String("f", "./DataDef.ts", "DataDef输出路径")
var mgrFile = flag.String("c", "./ConfigMgr.ts", "ConfigMgr输出路径")

func main() {
	flag.Parse()
	bytes, err := os.ReadFile(*fileName)
	if err != nil {
		fmt.Println(err)
		return
	}
	excels := DeserializationExcelJson(bytes)

	fileContent := ""

	keys := make([]string, 0, 10)
	types := make([]string, 0, 10)
	// keys = append(keys, "configfm")

	for _, e := range excels {
		keys = append(keys, e.Item)
		types = append(types, firstUpper(e.Item)+"Data")
		class := "export class " + firstUpper(e.Item) + "Data {\n"
		for key, value := range e.Schema {

			keyType := ""
			values := value.([]interface{})
			comment := values[1].(string)

			if reflect.TypeOf(values[0]).Kind() == reflect.Array || reflect.TypeOf(values[0]).Kind() == reflect.Slice {
				keyType = fmt.Sprintf("Array<%s>\n", values[0].([]interface{})[0])
			} else {
				keyType = values[0].(string)
			}

			if strings.Contains(keyType, "map[") {
				startIndex := strings.Index(keyType, "map[")
				endIndex := strings.LastIndex(keyType, "]")
				mapOrgStr := keyType[startIndex:endIndex]

				strArr := strings.Split(mapOrgStr, " ")
				mapStr := ""
				for _, mapItem := range strArr {

					mapItemValues := strings.Split(mapItem, ":")
					for i, mapItemValue := range mapItemValues {
						mapItemValue1 := strings.Replace(strings.Replace(mapItemValue, "[", "", 1), "]", "", -1)
						mapStr += mapItemValue1
						if i == 0 {
							mapStr += ":"
						}
					}
					mapStr += ","
				}

				mapStr = mapStr[3 : len(mapStr)-1]
				mapStr = "{" + mapStr + "}"
				keyType = strings.Replace(keyType, mapOrgStr+"]", mapStr, 1)
			}

			keyType = strings.Replace(keyType, "bool", "boolean", -1)
			keyType = strings.Replace(keyType, "double", "number", -1)
			keyType = strings.Replace(keyType, "int", "number", -1)
			keyType = strings.Replace(keyType, "float", "number", -1)
			keyType = strings.Replace(keyType, "\n", "", -1)

			// fmt.Printf("keyType %s \n", keyType);
			class += fmt.Sprintf("\n\t/** %s **/\n\t%s:%s;", comment, key, keyType)

		}
		class += "\n}"
		fileContent += class + "\n\n"
	}

	dir, _ := filepath.Split(*outFile)
	exist, err := pathExists(dir)
	if err != nil {
		fmt.Println(err)
		return
	}

	if !exist {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	if err := os.WriteFile(*outFile, []byte(fileContent), 0777); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("代码生成完成,路径 %s \n", *outFile)
	writeConfigMgr(keys, types)

	fmPath := "../assets/resources/config/framework/fm.json"
	if b, _ := pathExists(fmPath); !b {
		return
	}

	bytes, err = os.ReadFile(fmPath)
	if err != nil {
		fmt.Println(err)
		return
	}

	fm := DeserializationFMJson(bytes)
	baseURL := "/"
	for _, name := range keys {
		found := false
		for _, c := range fm.ConfigsNeedPreload {
			if c.Name == name {
				found = true
				break
			}
		}

		if found {
			continue
		}
		config := Configs{
			Name:        name,
			Url:         baseURL + name + ".json",
			LocalUrl:    "config/" + name,
			LocalPrefer: true,
		}

		fm.ConfigsNeedPreload = append(fm.ConfigsNeedPreload, config)
	}

	bytes, _ = json.Marshal(fm)
	var str bytes1.Buffer
	err1 := json.Indent(&str, bytes, "", "\t")
	if err1 != nil {
		fmt.Println(err1)
		return
	}

	if err := os.WriteFile(fmPath, str.Bytes(), 0777); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("fm.json配置完成")
}

func writeConfigMgr(keys []string, types []string) {

	keyStr := ""
	for _, v := range keys {
		keyStr += "\"" + v + "\"" + ","
	}
	keyStr = keyStr[0 : len(keyStr)-1]
	typeStr := strings.Join(types, ",")

	fileContent := `//自动生成
import { Asset, assetManager, AssetManager, JsonAsset, log, resources } from "cc";
import { ${type} } from "./DataDef";

/**
 * 通用配置
 */
export default class ConfigMgr {
    private keyMap: Map<Function, string> = new Map();
    private dataMap: Map<string, Record<string, any>> = new Map();
    private frames = {};
    private loadState = 0;
    private waitQueues: Array<() => void> = [];

    private static instance: ConfigMgr;

    static get Ins() {
        return this.GetInstance();
    }
    public static GetInstance(): ConfigMgr {
        if (!ConfigMgr.instance) {
            ConfigMgr.instance = new ConfigMgr();
        }
        return ConfigMgr.instance;
    }
    private constructor() {
        const keys = [${keys}];
        const constructors = [${type}];

        for (let i = 0; i < keys.length; i++) this.keyMap.set(constructors[i], keys[i]);
    }

    /**
     * 获取所有数据
     * @param option
     */
    public LoadAll(option?: { remoteUrl: string; remoteKeys?: Array<string> }): Promise<void> {
        return new Promise((resolve) => {
            if (this.loadState == 2) {
                resolve();
                return;
            }

            if (this.loadState == 1) {
                this.waitQueues.push(resolve);
                return;
            }

            this.waitQueues.push(resolve);

            let count = this.keyMap.size;
            const loadPath = "config/";
            this.loadState = 1;
            this.keyMap.forEach((key, value) => {
                this.LoadRes(
                    loadPath + key,
                    null,
                    (err, asset: JsonAsset) => {
                        if (err) console.log(err);
                        else this.dataMap.set(key, asset.json);

                        count--;

                        if (count <= 0) {
                            this.loadState = 2;
                            while (this.waitQueues.length > 0) {
                                const call = this.waitQueues.shift();
                                call();
                            }
                        }
                    },
                    this
                );
            });
        });
    }
    /////////////////////////////////////////////////////////////////////////////////////////////////////////////////
    private LoadRes(
        url: string,
        bundleName?: string,
        finishCall?: (err: Error, asset: any) => void,
        caller?: any,
        assetType?: typeof Asset
    ) {
        let callback = (err: Error, asset: any) => {
            if (!err) {
                if (assetType?.name == "cc_SpriteFrame") {
                    if (asset.shift) {
                        asset.forEach((element) => {
                            // cc.log(element)
                            this.frames[url + element.name] = element;
                        });
                    } else this.frames[url] = asset;
                }
            }
            finishCall?.call(caller, err, asset);
        };

        if (bundleName) this.LoadResFromBundle(url, bundleName, callback, assetType);
        else this.LoadResFromResources(url, callback, assetType);
    }

    private LoadResFromResources(
        url: string,
        onComplete: (err: Error, asset: any) => void,
        assetType?: typeof Asset
    ) {
        if (this.IsDir(url)) resources.loadDir(url, assetType, onComplete);
        else resources.load(url, assetType, onComplete);
    }

    private LoadResFromBundle(
        url: string,
        bundleName: string,
        onComplete?: (err: Error, asset: any) => void,
        assetType?: typeof Asset
    ) {
        let tmpBundle = assetManager.getBundle(bundleName);
        if (tmpBundle) {
            if (this.IsDir(url)) tmpBundle.loadDir(url, assetType, onComplete);
            else tmpBundle.load(url, assetType, onComplete);
        } else {
            assetManager.loadBundle(bundleName, (err: Error, bundle: AssetManager.Bundle) => {
                if (!err) {
                    if (this.IsDir(url)) tmpBundle.loadDir(url, assetType, onComplete);
                    else bundle.load(url, assetType, onComplete);
                } else {
                    log("loadBundle err", err);
                }
            });
        }
    }

    private IsDir(url: string) {
        return url[url.length - 1] == "/";
    }
    /////////////////////////////////////////////////////////////////////////////////////////////////////////////////
    /**
     * 用于单个数据,通常用于系统数据，只有单条数据
     * @param type 类型
     * @returns
     */
    public GetOne<T>(type: new () => T): T {
        const key = this.keyMap.get(type);
        return this.dataMap.get(key) as T;
    }

    /**
     * 获取所有数据 Array形式（不推荐对record配置使用，但也能用）
     * @param type 类型
     * @returns
     */
    public GetList<T>(type: new () => T): Array<T> {
        const key = this.keyMap.get(type);
        const data = this.dataMap.get(key);
        if (data instanceof Array) {
            return data as Array<T>;
        } else {
            console.warn("getList<T>: data is Object", key);
            const confs: Record<string, any>[] = new Array<Record<string, any>>();
            const keys = Object.keys(data);
            keys.forEach((key) => {
                confs.push(data[key]);
            });
            return confs as Array<T>;
        }
    }

    /**
     * 获取所有数据 Record形式
     * @param type 类型
     * @returns
     */
    public GetRecord<T>(type: new () => T): Record<string, T> {
        const key = this.keyMap.get(type);
        const data = this.dataMap.get(key);
        if (data instanceof Array) {
            console.error("getRecord<T> fail: data is Array", key);
            return;
        }
        return data as Record<string, T>;
    }

    /**
     * 获取数据数量
     * @param type 类型
     * @returns
     */
    public GetLength<T>(type: new () => T): number {
        const key = this.keyMap.get(type);
        const data = this.dataMap.get(key);
        if (data instanceof Array) {
            return (data as Array<T>).length;
        } else {
            return Object.keys(data).length;
        }
    }

    /**
     * 根据key获取数据
     * @param key object key
     * @param type 类型
     * @returns
     */
    public GetByKey<T>(
        key: string | number,
        type: new () => T,
        isLog: boolean = true
    ): T | undefined {
        if (typeof key === "number") {
            key = key.toString();
        }
        const dataKey = this.keyMap.get(type);
        const data = this.dataMap.get(dataKey);
        if (data.hasOwnProperty(key)) {
            return data[key] as T;
        } else {
            if (isLog) {
                console.error("ConfigMgr.getByKey fail", dataKey, key);
            }
            return undefined;
        }
    }

    /**
     * 根据index获取数据
     * @param index 配置数组索引
     * @param type 类型
     * @returns
     */
    public GetByIndex<T>(index: number, type: new () => T): T | undefined {
        const datas = this.GetList(type);
        if (index >= 0 && index < datas.length) {
            return datas[index];
        } else {
            console.error("ConfigMgr.getByIndex fail", this.keyMap.get(type), index);
            return undefined;
        }
    }

    public Find<T>(type: new () => T, predicate: (value: T) => unknown): T {
        const key = this.keyMap.get(type);
        const data = this.dataMap.get(key);
        if (data instanceof Array) {
            return (data as Array<T>)?.find(predicate);
        } else {
            const record = data as Record<string, T>;
            for (const key in record) {
                if (predicate(record[key])) {
                    return record[key];
                }
            }
            return undefined;
        }
    }

    public Filter<T>(type: new () => T, predicate: (value: T) => unknown): Array<T> {
        const key = this.keyMap.get(type);
        const data = this.dataMap.get(key);
        if (data instanceof Array) {
            return (data as Array<T>)?.filter(predicate);
        } else {
            const record = data as Record<string, T>;
            const result: T[] = [];
            for (const key in record) {
                if (predicate(record[key])) {
                    result.push(record[key]);
                }
            }
            return result;
        }
    }

    /** 数据大都是在一起的，从找到开始，到找不到结束，优化搜索性能 */
    public FilterQuick<T>(type: new () => T, predicate: (value: T) => unknown): Array<T> {
        const key = this.keyMap.get(type);
        const data = this.dataMap.get(key);
        if (data instanceof Array) {
            const datas: Array<T> = [];
            const ary = data as Array<T>;
            let isStart = false;
            for (let i = 0; i < ary.length; i++) {
                const conf = ary[i];
                if (predicate(conf)) {
                    isStart = true;
                    datas.push(conf);
                } else if (isStart) {
                    break;
                }
            }
            return datas;
        } else {
            const record = data as Record<string, T>;
            const datas: Array<T> = [];
            let isStart = false;
            for (const key in record) {
                const conf = record[key];
                if (predicate(conf)) {
                    isStart = true;
                    datas.push(conf);
                } else if (isStart) {
                    break;
                }
            }
            return datas;
        }
    }

    public ForEach<T>(type: new () => T, callbackfn: (value: T) => void): void {
        const key = this.keyMap.get(type);
        const data = this.dataMap.get(key);
        if (data instanceof Array) {
            (data as Array<T>)?.forEach(callbackfn);
        } else {
            const record = data as Record<string, T>;
            for (const key in record) {
                callbackfn(record[key]);
            }
        }
    }
}`

	fileContent = strings.ReplaceAll(fileContent, "${keys}", keyStr)
	fileContent = strings.ReplaceAll(fileContent, "${type}", typeStr)

	dir, _ := filepath.Split(*mgrFile)
	exist, err := pathExists(dir)
	if err != nil {
		fmt.Println(err)
		return
	}

	if !exist {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	if err := os.WriteFile(*mgrFile, []byte(fileContent), 0777); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("ConfigMgr 代码生成完成,路径 %s \n", *mgrFile)
}

// DeserializationExcelJson 反序列化Excel.json
func DeserializationExcelJson(bytes []byte) []Excel {
	excels := make([]Excel, 0)
	err := json.Unmarshal(bytes, &excels)
	if err != nil {
		panic(err)
	}
	return excels
}

// DeserializationFMJson 反序列化FM.json
func DeserializationFMJson(bytes []byte) *FM {
	var fm FM
	err := json.Unmarshal(bytes, &fm)
	if err != nil {
		panic(err)
	}
	return &fm
}

// 首字母大写
func firstUpper(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// 判断文件或文件夹是否存在
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
