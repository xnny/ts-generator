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
import { JsonAsset } from "cc";
import Singleton from "../Framework/GOF/Singleton/Singleton";
import ResManager from "../Singletons/ResManager";
import { ${type} } from "./DataDef";

/**
 * 通用配置
 */
export default class ConfigMgr extends Singleton {
    private keyMap: Map<Function, string> = new Map();
    private dataMap: Map<string, Record<string, any>> = new Map();
    private dataListMap: Map<string, Record<string, any>[]> = new Map();

    constructor() {
        super();
        const keys = [${keys}];
        const constructors = [${type}];

        for (let i = 0; i < keys.length; i++) this.keyMap.set(constructors[i], keys[i]);
    }
    static get Ins() {
        return super.GetInstance<ConfigMgr>();
    }

    private loadState = 0;
    private waitQueues: Array<() => void> = [];

    /**
     * 获取所有数据
     * @param option
     */
    public loadAll(option?: { remoteUrl: string; remoteKeys?: Array<string> }): Promise<void> {
        this.dataListMap.clear();
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
                ResManager.Ins.loadRes(
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

    /**
     * 用于单个数据,通常用于系统数据，只有单条数据
     * @param type 类型
     * @returns
     */
    public getOne<T>(type: new () => T): T {
        const key = this.keyMap.get(type);
        return this.dataMap.get(key) as T;
    }

    /**
     * 获取单个
     * @param keyVal key值
     * @param type 类型
     * @param key key
     * @returns
     */
    public getById<T>(keyVal: number | string, type: new () => T, key = "id"): T {
        return this.find(type, (p) => p[key] == keyVal);
    }

    /**
     * 获取所有数据
     * @param type 类型
     * @returns
     */
    public getAll<T>(type: new () => T): Array<T> {
        const key = this.keyMap.get(type);
        const data = this.dataMap.get(key);
        if (data instanceof Array) {
            return data as Array<T>;
        } else {
            if (!this.dataListMap.has(key)) {
                const confs: Record<string, any>[] = new Array<Record<string, any>>();
                const keys = Object.keys(data);
                keys.forEach((key) => {
                    confs.push(data[key]);
                });
                this.dataListMap.set(key, confs);
            }
            return this.dataListMap.get(key) as Array<T>;
        }
    }

    /**
     * 获取所有数据的List
     * @param type 类型
     * @returns
     */
    public getList<T>(type: new () => T): Array<T> {
        const key = this.keyMap.get(type);
        return this.dataMap.get(key) as Array<T>;
    }

    /**
     * 根据key获取数据
     * @param type 类型
     * @returns
     */
    public getByKey<T>(keyVal: string, type: new () => T): T {
        const key = this.keyMap.get(type);
        return this.dataMap.get(key)[keyVal] as T;
    }

    public find<T>(
        type: new () => T,
        predicate: (value: T, index: number, obj: T[]) => unknown
    ): T {
        return this.getAll(type)?.find(predicate);
    }

    public filter<T>(
        type: new () => T,
        predicate: (value: T, index: number, array: T[]) => unknown
    ): Array<T> {
        return this.getAll(type)?.filter(predicate);
    }

    public forEach<T>(
        type: new () => T,
        callbackfn: (value: T, index: number, array: T[]) => void
    ): void {
        this.getAll(type)?.forEach(callbackfn);
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
