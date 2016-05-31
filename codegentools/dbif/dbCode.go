package main

import (
	"fmt"
	"go/ast"
	"os"
	"strings"
)

var fileHeader = `package models
import (
   "fmt"
   "encoding/json"
   "github.com/garyburd/redigo/redis"
   "reflect"
   "errors"
)

//Dummy import
var _ = redis.Args{}
var _ = errors.New("")

`

var fileHeaderForState = `package models
import (
   "fmt"
   "github.com/garyburd/redigo/redis"
   "errors"
//   "strings"
`
var endFileHeaderState = `)
//Dummy import
var _ = redis.Args{}
var _ = errors.New("")
var _ = fmt.Sprintln("")

`
var goBasicTypesMap = map[string]bool{
	"bool":    true,
	"uint8":   true,
	"uint16":  true,
	"uint32":  true,
	"uint64":  true,
	"string":  true,
	"float64": true,
	"int8":    true,
	"int16":   true,
	"int32":   true,
	"int64":   true,
}

var goTypeToRedisTypeMap = map[string]string{
	"bool":    "Bool",
	"uint":    "Uint64",
	"uint8":   "Uint64",
	"uint16":  "Uint64",
	"uint32":  "Uint64",
	"uint64":  "Uint64",
	"int":     "Int64",
	"int8":    "Int64",
	"int16":   "Int64",
	"int32":   "Int64",
	"int64":   "Int64",
	"string":  "String",
	"float64": "Float64",
}

func (obj *ObjectInfoJson) WriteStoreObjectInDBFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) {
	var lines []string
	lines = append(lines, "\nfunc (obj "+obj.ObjName+") StoreObjectInDb(dbHdl redis.Conn) error {\n")
	lines = append(lines,
		`_, err := dbHdl.Do("HMSET", redis.Args{}.Add(obj.GetKey()).AddFlat(obj)...) 
		if err != nil {
			fmt.Println("Failed to store object in DB", obj)
			return err
		}`)
	// Write Secondary table lines
	secondaryLines := obj.WriteSecondaryTableInsertIntoDBFcn(str, fd, attrMap, objMap)
	if len(secondaryLines) > 0 {
		lines = append(lines, secondaryLines...)
	}
	lines = append(lines, "\nreturn nil\n}")
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}

func (obj *ObjectInfoJson) WriteSecondaryTableInsertIntoDBFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) []string {
	var lines []string
	if strings.HasPrefix(obj.ObjName, "Vxlan") { // Temporary hack. Need to fix it. Hari. TODO
		return lines
	}
	for _, attrInfo := range attrMap {
		if attrInfo.IsArray == true {
			if _, ok := goBasicTypesMap[attrInfo.VarType]; !ok {
				lines = append(lines, `
					bytes, err := json.Marshal(obj.`+attrInfo.MemberName+`)
					if err != nil {
						fmt.Println("Failed to marshal struct when storing object in DB", obj)
						return err
					}
					_, err = dbHdl.Do("SET", obj.GetKey()+"`+attrInfo.MemberName+`", string(bytes))
					if err != nil {
						fmt.Println("Failed to store object in DB", obj)
						return err
					}`)
			} else {
				//Member is a slice of native data type elements
				lines = append(lines, `
					for idx := len(obj.`+attrInfo.MemberName+`) - 1; idx >= 0; idx-- {
						_, err := dbHdl.Do("LPUSH", obj.GetKey()+"`+attrInfo.MemberName+`", obj.`+attrInfo.MemberName+`[idx])
						if err != nil {
							fmt.Println("Failed to store slice member in DB", obj)
							return err
						}
					}`)
			}
		}
	}
	return lines
}

func (obj *ObjectInfoJson) WriteDeleteObjectFromDbFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) {
	var lines []string
	lines = append(lines, "\nfunc (obj "+obj.ObjName+") DeleteObjectFromDb(dbHdl redis.Conn) error {\n")
	//Delete primary key
	lines = append(lines,
		`_, err := dbHdl.Do("DEL", obj.GetKey()) 
		if err != nil {
			fmt.Println("Failed to delete obj from DB", obj)
			return err
		}`)
	//Delete key corresponding to secondary entries if any
	for _, attrInfo := range attrMap {
		if attrInfo.IsArray == true {
			lines = append(lines, `
				_, err = dbHdl.Do("DEL", obj.GetKey()+"`+attrInfo.MemberName+`")
				if err != nil {
					fmt.Println("Failed to delete secondary table from DB", obj)
					return err
				}`)
		}
	}
	lines = append(lines, `
		return nil 
	}`)
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}

func (obj *ObjectInfoJson) WriteGetObjectFromDbFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) {
	var lines []string
	var firstListOfStructs, firstList bool = true, true
	lines = append(lines, "\nfunc (obj "+obj.ObjName+") GetObjectFromDb(objKey string, dbHdl redis.Conn) (ConfigObj, error) {\n")
	lines = append(lines, "var object "+obj.ObjName+"\n")
	lines = append(lines,
		`val, err := redis.Values(dbHdl.Do("HGETALL", objKey))
		if err != nil || len(val) == 0 {
			fmt.Println("Failed to get obj from DB", obj)
			return object, errors.New("Failed to get obj from DB")
		}
		_ = redis.ScanStruct(val, &object)
		`)
	/*
		if err != nil {
			fmt.Println("Failed to construct config obj from DB data", obj)
			return object, err
		}`)*/
	for _, attrInfo := range attrMap {
		if attrInfo.IsArray == true {
			if _, ok := goBasicTypesMap[attrInfo.VarType]; !ok {
				if firstListOfStructs {
					lines = append(lines, "\nvar strVal string\n")
					firstListOfStructs = false
				}
				//Member is a slice of structs
				lines = append(lines, `
				    strVal, err = redis.String(dbHdl.Do("GET", objKey+"`+attrInfo.MemberName+`"))
					if err != nil {
						fmt.Println("Failed to get obj from DB data", obj)
						return object, err
					}
					err = json.Unmarshal([]byte(strVal), &object.`+attrInfo.MemberName+`)
					if err != nil {
						fmt.Println("Failed to unmarshal db object", obj)
						return object, err
					}`)
			} else {
				if firstList {
					lines = append(lines, "\nvar idx, listLen int\n")
					firstList = false
				}
				//Member is a slice of native data type elements
				lines = append(lines, `
				    listLen, err = redis.Int(dbHdl.Do("LLEN", objKey+"`+attrInfo.MemberName+`"))
					if err != nil {
						fmt.Println("Failed to retrieve list len for secondary table", obj)
						return object, err
					}
					for idx = 0; idx < listLen; idx++ {
						val, err := redis.`+goTypeToRedisTypeMap[attrInfo.VarType]+`(dbHdl.Do("LINDEX", objKey+"`+attrInfo.MemberName+`",idx))
						if err != nil {
							fmt.Println("Failed to reconstruct list for secondary table", obj)
							return object, err
						}
						object.`+attrInfo.MemberName+` = append(object.`+attrInfo.MemberName+`, `+attrInfo.VarType+`(val))
					}`)
			}
		}
	}
	lines = append(lines, "\nreturn object, nil\n}")
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}

func (obj *ObjectInfoJson) IsNumericType(typeVal string) bool {
	switch typeVal {
	case "uint8", "uint16", "uint32", "uint64", "int8", "int16", "int32", "int64", "float32", "float64", "complex64", "complex128", "byte", "rune":
		return true
	default:
		return false
	}
	return false
}

func (obj *ObjectInfoJson) WriteKeyRelatedFcns(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) {
	var lines []string
	lines = append(lines, "\nfunc (obj "+obj.ObjName+") GetKey() string {\n")
	numKeys := 0
	keyStr := `key := "` + obj.ObjName + `#"`
	for _, fld := range str.Fields.List {
		if fld.Names != nil {
			switch fld.Type.(type) {
			case *ast.Ident:
				varName := fld.Names[0].String()
				if fld.Tag != nil {
					if strings.Contains(fld.Tag.Value, "SNAPROUTE") {
						idntType := fld.Type.(*ast.Ident)
						varType := idntType.String()
						if numKeys == 0 {
							if obj.IsNumericType(varType) {
								keyStr = keyStr + "+ fmt.Sprintf(\"%d\", obj." + varName + ")"
							} else {
								keyStr = keyStr + "+ obj." + varName
							}
						} else {
							if obj.IsNumericType(varType) {
								keyStr = keyStr + "+ \"#\" + fmt.Sprintf(\"%d\", obj." + varName + ")"
							} else {
								keyStr = keyStr + "+ \"#\" + obj." + varName
							}
						}
						numKeys += 1
					}
				}
			}
		}
	}
	lines = append(lines, keyStr, `
		return key
		}`)
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}

func (obj *ObjectInfoJson) WriteGetAllObjFromDbFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) {
	var lines []string
	lines = append(lines, "\nfunc (obj "+obj.ObjName+") GetAllObjFromDb(dbHdl redis.Conn) (objList []ConfigObj, err error) { \n")
	lines = append(lines,
		`keyStr := "`+obj.ObjName+`#*"
		keys, err := redis.Strings(dbHdl.Do("KEYS", keyStr))
		if err != nil {
			fmt.Println("Failed to get all object keys from db", obj)
			return nil, err
		}
		for idx := 0; idx < len(keys); idx++ {
		keyType, err := redis.String(dbHdl.Do("Type", keys[idx]))
		if err != nil {
			fmt.Println("Error getting keyType")
			return nil, err
		}
		if keyType != "hash" {
			fmt.Println("Do not process list object")
			continue
		}
			object, err := obj.GetObjectFromDb(keys[idx], dbHdl)
			if err != nil {
				fmt.Println("Failed to get object from db", obj)
				return nil, err
			}
			objList = append(objList, object)
		}
		return objList, nil
	}`)
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}

//FIXME: GetBulk for secondary table will be implemented as part of actual GetBulk implementation
/*
func (obj *ObjectInfoJson) WriteGetBulkSecondaryTableFromDBFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) []string {
	var lines []string
	//if !strings.Contains(obj.ObjName, "Policy") { // Temporary hack. Need to fix it. Hari. TODO
	if strings.HasPrefix(obj.ObjName, "Vxlan") { // Temporary hack. Need to fix it. Hari. TODO
		return lines
	}
	lines = append(lines, "var frnKey string\n")
	for _, attrInfo := range attrMap {
		if attrInfo.IsArray == true {
			lines = append(lines, "// Fetch values for "+attrInfo.MemberName+" attribute\n")
			lines = append(lines, "secondaryObj"+attrInfo.MemberName+"Map := make(map[", keyType, "][] "+attrInfo.VarType+" ) \n")
			objName := "secObj" + attrInfo.MemberName
			lines = append(lines, " var "+objName+" "+attrInfo.VarType+"\n")
			var attrs []string
			count := 0
			if _, ok := goTypesToSqliteMap[attrInfo.VarType]; !ok {
				memberAttrMap := getObjectMemberInfo(objMap, attrInfo.VarType)
				count = len(memberAttrMap)
				attrs = make([]string, count)
				for name, val := range memberAttrMap {
					attrs[val.Position] = name
				}
			} else {
				attrs = append(attrs, attrInfo.MemberName)
			}
			dbCmdStr := "dbCmd = \"select * from " + obj.ObjName + attrInfo.MemberName + "\""
			lines = append(lines, dbCmdStr+"\n")
			lines = append(lines, `
						rows, err = dbHdl.Query(dbCmd)
						if err != nil {
						 return err, 0, 0, false, nil
						 }
						defer rows.Close()`+"\n")
			lines = append(lines, " for rows.Next() { \n")
			stmt := "if err = rows.Scan( &frnKey,"
			for idx, attr := range attrs {
				if idx != len(attrs)-1 {
					if _, ok := goTypesToSqliteMap[attrInfo.VarType]; !ok {
						stmt = stmt + "&" + objName + "." + attr + ", "
					} else {
						stmt = stmt + "&" + objName + ", "
					}
				} else {
					if _, ok := goTypesToSqliteMap[attrInfo.VarType]; !ok {
						stmt = stmt + "&" + objName + "." + attr + "); err != nil {\n"
					} else {
						stmt = stmt + "&" + objName + "); err != nil {\n"
					}
				}
			}
			lines = append(lines, stmt)
			lines = append(lines, `fmt.Println("Db method Scan failed when iterating over `+obj.ObjName+attrInfo.MemberName+`")`+"\n")
			lines = append(lines, `return err, 0, 0, false, nil`+"\n } \n")
			//lines = append(lines, arrayName +" = append("+arrayName + "," + objName +"  )\n } \n")
			lines = append(lines, "if secondaryObj"+attrInfo.MemberName+"Map[frnKey]== nil {\n")
			lines = append(lines, "secondaryObj"+attrInfo.MemberName+"Map[frnKey] = make([]"+attrInfo.VarType+", 0)\n")
			lines = append(lines, "}\n")
			lines = append(lines, "secondaryObj"+attrInfo.MemberName+"Map[frnKey]  = append("+"secondaryObj"+attrInfo.MemberName+"Map[frnKey] ,"+objName+"  )\n } \n")
			//lines = append(lines, "secondaryObj" + attrInfo.MemberName + "Map[frnKey]=" + arrayName+"\n")
			lines = append(lines, "\n")
		}
	}
	return lines
}
*/

//FIXME: GetBulk is currently implemented to call GetAllObj
func (obj *ObjectInfoJson) WriteGetBulkObjFromDbFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) {
	var lines []string
	lines = append(lines, "\nfunc (obj "+obj.ObjName+") GetBulkObjFromDb(startIndex int64, count int64, dbHdl redis.Conn) (err error, objCount int64, nextMarker int64, moreExist bool, objList []ConfigObj) { \n")
	lines = append(lines,
		`objList, err = obj.GetAllObjFromDb(dbHdl)
		if err != nil {
			fmt.Println("Failed to get all object from db", obj)
			return err, 0, 0, false, nil
		}
		return nil, int64(len(objList)), int64(0), false, objList
		}`)
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}

func (obj *ObjectInfoJson) WriteCompareObjectsAndDiffFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) {
	var lines []string
	lines = append(lines, "\nfunc (obj "+obj.ObjName+") CompareObjectsAndDiff(updateKeys map[string]bool, inObj ConfigObj) ([]bool, error) {\n")
	lines = append(lines, "dbObj := inObj.("+obj.ObjName+")")
	lines = append(lines, `
			objTyp := reflect.TypeOf(obj)
			objVal := reflect.ValueOf(obj)
			dbObjVal := reflect.ValueOf(dbObj)
			attrIds := make([]bool, objTyp.NumField())
			idx := 0
			for i := 0; i < objTyp.NumField(); i++ {
				fieldTyp := objTyp.Field(i)
				if fieldTyp.Anonymous {
					continue
				}

				objVal := objVal.Field(i)
				dbObjVal := dbObjVal.Field(i)
				if _, ok := updateKeys[fieldTyp.Name]; ok {
					if objVal.Kind() == reflect.Int {
						if int(objVal.Int()) != int(dbObjVal.Int()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Int8 {
						if int8(objVal.Int()) != int8(dbObjVal.Int()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Int16 {
						if int16(objVal.Int()) != int16(dbObjVal.Int()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Int32 {
						if int32(objVal.Int()) != int32(dbObjVal.Int()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Int64 {
						if int64(objVal.Int()) != int64(dbObjVal.Int()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Uint {
						if uint(objVal.Uint()) != uint(dbObjVal.Uint()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Uint8 {
						if uint8(objVal.Uint()) != uint8(dbObjVal.Uint()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Uint16 {
						if uint16(objVal.Uint()) != uint16(dbObjVal.Uint()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Uint32 {
						if uint16(objVal.Uint()) != uint16(dbObjVal.Uint()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Uint64 {
						if uint16(objVal.Uint()) != uint16(dbObjVal.Uint()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Bool {
						if bool(objVal.Bool()) != bool(dbObjVal.Bool()) {
							attrIds[idx] = true
						}
					} else if objVal.Kind() == reflect.Slice {
						attrIds[idx] = true
					} else {
						if objVal.String() != dbObjVal.String() {
							attrIds[idx] = true
						}
					}
					if attrIds[idx] {
						fmt.Println("attribute changed ", fieldTyp.Name)
					}
				}
				idx++

			}
			return attrIds[:idx], nil
		}

		`)
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}

func (obj *ObjectInfoJson) WriteUpdateObjectInDbFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) {
	var lines []string
	lines = append(lines, "\nfunc (obj "+obj.ObjName+") UpdateObjectInDb(inObj ConfigObj, attrSet []bool, dbHdl redis.Conn) error {\n")
	lines = append(lines,
		`_, err := dbHdl.Do("HMSET", redis.Args{}.Add(obj.GetKey()).AddFlat(obj)...) 
		if err != nil {
			fmt.Println("Failed to store object in DB", obj)
			return err
		}`)
	lines = append(lines, `
						objTyp := reflect.TypeOf(obj)
						objVal := reflect.ValueOf(obj)
						idx := 0
						for i := 0; i < objTyp.NumField(); i++ {
							if fieldTyp := objTyp.Field(i); fieldTyp.Anonymous {
								continue
							}
							if attrSet[idx] {
								fieldTyp := objTyp.Field(i)
								fieldVal := objVal.Field(i)
								fieldName := fieldTyp.Name
								if fieldVal.Kind() == reflect.Slice {
									if fieldVal.Len() > 0 {
										secObjVal := fieldVal.Index(0)
										_, err := dbHdl.Do("DEL", obj.GetKey()+fieldName)
										if err != nil {
											return err
										}
										if secObjVal.Kind() == reflect.Struct {
											bytes, err := json.Marshal(fieldVal.Interface())
											if err != nil {
												return err
											}
											_, err = dbHdl.Do("SET", obj.GetKey()+fieldName, string(bytes))
											if err != nil {
												return err
											}
										} else {
											for idx := fieldVal.Len() - 1; idx >= 0; idx-- {
												_, err := dbHdl.Do("LPUSH", obj.GetKey()+fieldName, fieldVal.Index(idx))
												if err != nil {
													return err
												}
											}
										}
									}
								}
							}
							idx++
						}
						return nil
					}`)
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}
func (obj *ObjectInfoJson) WriteCopyRecursiveFcn(str *ast.StructType, fd *os.File) {
	var lines []string
	lines = append(lines, "\nfunc (obj "+obj.ObjName+")")
	lines = append(lines, ` CopyRecursive(dest, src reflect.Value) {
	                       fmt.Println("copyRecursive")
	                       switch src.Kind() {
	                           case reflect.Slice:
		                       fmt.Println("Slice")
		                       dest.Set(reflect.MakeSlice(src.Type(), src.Len(), src.Cap()))
		                       for i := 0; i < src.Len(); i++ { 
	                               obj.CopyRecursive(src.Index(i), dest.Index(i))
	                           }
	                           case reflect.Struct:
		                       fmt.Println("struct")
		                       for i := 0; i < src.NumField(); i++ {
                                    obj.CopyRecursive(src.Field(i), dest.Field(i))
	                          }
	                           case reflect.String:
		                       dest.SetString(src.Interface().(string))
 	                           case reflect.Int:
		                       dest.SetInt(int64(src.Interface().(int)))
	                           case reflect.Bool:
		                       dest.SetBool(src.Interface().(bool))
	                           case reflect.Float64:
		                       dest.SetFloat(src.Interface().(float64))
	                           default:
		                       dest.Set(src)
	                       }
                       }`)
	lines = append(lines, "\n")
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}
func (obj *ObjectInfoJson) WriteMergeDbAndConfigObjForPatchUpdateFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) {
	var lines []string
	lines = append(lines, "\nfunc (obj "+obj.ObjName+") MergeDbAndConfigObjForPatchUpdate(dbObj ConfigObj, patchOpInfoSlice []PatchOpInfo) (ConfigObj, []bool, error) {\n")
	lines = append(lines, "var mergedObject, tempObject  "+obj.ObjName+"\n")
	lines = append(lines, `objTyp := reflect.TypeOf(obj)
						dbObjVal := reflect.ValueOf(dbObj)
						mergedObjVal := reflect.ValueOf(&mergedObject)
	                      diff := make([]bool, objTyp.NumField())
	                      for i := 0; i < objTyp.NumField(); i++ {
							fieldTyp := objTyp.Field(i)
		                      if fieldTyp.Anonymous {
			                      continue
		                      }
		                      dbObjField := dbObjVal.Field(i)
							if dbObjField.Kind() == reflect.Int ||
								dbObjField.Kind() == reflect.Int8 ||
								dbObjField.Kind() == reflect.Int16 ||
								dbObjField.Kind() == reflect.Int32 ||
								dbObjField.Kind() == reflect.Int64 {
								mergedObjVal.Elem().Field(i).SetInt(dbObjField.Int())
							} else if dbObjField.Kind() == reflect.Uint ||
								dbObjField.Kind() == reflect.Uint ||
								dbObjField.Kind() == reflect.Uint8 ||
								dbObjField.Kind() == reflect.Uint16 ||
								dbObjField.Kind() == reflect.Uint32 {
								mergedObjVal.Elem().Field(i).SetUint(dbObjField.Uint())
							} else if dbObjField.Kind() == reflect.Bool {
								mergedObjVal.Elem().Field(i).SetBool(dbObjField.Bool())
							} else if dbObjField.Kind() == reflect.Slice {
                                   obj.CopyRecursive(mergedObjVal.Elem().Field(i), dbObjField)
                              } else {
								mergedObjVal.Elem().Field(i).SetString(dbObjField.String())
							}
						}
       	                 for _, patchOpInfo := range patchOpInfoSlice {
		                     fmt.Println("path = ", patchOpInfo.Path, "op = ", patchOpInfo.Path, "value = ", patchOpInfo.Value)
		                     idx := 0
	                         for i := 0; i < objTyp.NumField(); i++ {
		                         fieldTyp := objTyp.Field(i)
		                         if fieldTyp.Anonymous {
			                        continue
		                         }
			                    if fieldTyp.Name == patchOpInfo.Path {
				                   fmt.Println("found pathStr ", patchOpInfo.Path, " at index ", i)
				                   diff[idx] = true
				                   switch patchOpInfo.Path {
				`)
	for _, attrInfo := range attrMap {
		attrStr := "\"" + attrInfo.MemberName + "\""
		lines = append(lines, "case "  + attrStr + ":\n")
		lines = append(lines,"err := json.Unmarshal([]byte(patchOpInfo.Value), &tempObject."+attrInfo.MemberName+")\n")
		lines = append(lines, `
						                  if err != nil {
							                 fmt.Println("error unmarshaling value:", err)
							                 return mergedObject, diff, errors.New(fmt.Sprintln("error unmarshaling value:", err))
						                  }
						                  switch patchOpInfo.Op {
						`)
		if attrInfo.IsArray{
             lines = append(lines,`
						                      case "add":
						   `)
		    lines = append(lines," for j := 0;j< len(tempObject."+attrInfo.MemberName+");j++ {\n")
		    lines = append(lines,"mergedObject."+attrInfo.MemberName+"= append(mergedObject."+attrInfo.MemberName+", tempObject."+attrInfo.MemberName+"[j])\n")
		    lines = append(lines,"}\n")
	         lines = append(lines, `
						                      case "remove":
						`)
			lines = append(lines, "for k := 0; k < len(tempObject."+attrInfo.MemberName+"); k++ {\n")
			lines = append(lines,`
							found := false
							match := -1
					`)
			lines = append(lines,	"for k2 := 0 ; k2 < len(mergedObject."+attrInfo.MemberName+");k2++{\n")
			lines = append(lines, "if mergedObject."+attrInfo.MemberName+"[k2] == tempObject."+attrInfo.MemberName+"[k] {")
			lines = append(lines,`
								    found = true
									match = k2 
									break
								}
							}
							if found {
					`)
			lines = append(lines, "mergedObject."+attrInfo.MemberName+"[match]  = mergedObject."+attrInfo.MemberName+"[len(mergedObject."+attrInfo.MemberName+") - 1]\n")
			lines = append(lines, "mergedObject."+attrInfo.MemberName+" = mergedObject."+attrInfo.MemberName+"[:(len(mergedObject."+attrInfo.MemberName+") - 1)]\n")
			lines = append(lines,`
							}
						}
						 `)
		}
	    lines = append(lines, `
						                      case "replace":
							                     fmt.Println("replace")
											default:				   
					                              fmt.Println("Patch update op not supported for this attr")
								                 return mergedObject, diff, errors.New("Invalid patch op type ")
				                           }
					    `)
	}
	lines = append(lines, `
	                                }
				                    break
			                     }
			                     idx++
		                      }
		                      fmt.Println("mergedObj = ", mergedObject)
	                     }
						return mergedObject , diff, nil
					}
					`)
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}
func (obj *ObjectInfoJson) WriteMergeDbAndConfigObjFcn(str *ast.StructType, fd *os.File, attrMap []ObjectMemberAndInfo, objMap map[string]ObjectInfoJson) {
	var lines []string
	lines = append(lines, "\nfunc (obj "+obj.ObjName+") MergeDbAndConfigObj(dbObj ConfigObj, attrSet []bool) (ConfigObj, error) {\n")
	lines = append(lines, "var mergedObject  "+obj.ObjName+"\n")
	lines = append(lines, `objTyp := reflect.TypeOf(obj)
						objVal := reflect.ValueOf(obj)
						dbObjVal := reflect.ValueOf(dbObj)
						mergedObjVal := reflect.ValueOf(&mergedObject)
						idx := 0
						for i := 0; i < objTyp.NumField(); i++ {
							if fieldTyp := objTyp.Field(i); fieldTyp.Anonymous {
								continue
							}

							objField := objVal.Field(i)
							dbObjField := dbObjVal.Field(i)
							if attrSet[idx] {
								if dbObjField.Kind() == reflect.Int ||
									dbObjField.Kind() == reflect.Int8 ||
									dbObjField.Kind() == reflect.Int16 ||
									dbObjField.Kind() == reflect.Int32 ||
									dbObjField.Kind() == reflect.Int64 {
									mergedObjVal.Elem().Field(i).SetInt(objField.Int())
								} else if dbObjField.Kind() == reflect.Uint ||
									dbObjField.Kind() == reflect.Uint8 ||
									dbObjField.Kind() == reflect.Uint16 ||
									dbObjField.Kind() == reflect.Uint32 ||
									dbObjField.Kind() == reflect.Uint64 {
									mergedObjVal.Elem().Field(i).SetUint(objField.Uint())
								} else if dbObjField.Kind() == reflect.Bool {
									mergedObjVal.Elem().Field(i).SetBool(objField.Bool())
								} else if dbObjField.Kind() == reflect.Slice {
                                         obj.CopyRecursive(mergedObjVal.Elem().Field(i), objField)
                                   } else {
									mergedObjVal.Elem().Field(i).SetString(objField.String())
								}
							} else {
								if dbObjField.Kind() == reflect.Int ||
									dbObjField.Kind() == reflect.Int8 ||
									dbObjField.Kind() == reflect.Int16 ||
									dbObjField.Kind() == reflect.Int32 ||
									dbObjField.Kind() == reflect.Int64 {
									mergedObjVal.Elem().Field(i).SetInt(dbObjField.Int())
								} else if dbObjField.Kind() == reflect.Uint ||
									dbObjField.Kind() == reflect.Uint ||
									dbObjField.Kind() == reflect.Uint8 ||
									dbObjField.Kind() == reflect.Uint16 ||
									dbObjField.Kind() == reflect.Uint32 {
									mergedObjVal.Elem().Field(i).SetUint(dbObjField.Uint())
								} else if dbObjField.Kind() == reflect.Bool {
									mergedObjVal.Elem().Field(i).SetBool(dbObjField.Bool())
								} else if dbObjField.Kind() == reflect.Slice {
                                     obj.CopyRecursive(mergedObjVal.Elem().Field(i), dbObjField)
                                   } else {
									mergedObjVal.Elem().Field(i).SetString(dbObjField.String())
								}
							}
							idx++

						}
						return mergedObject , nil
					}
					`)
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}

func (obj *ObjectInfoJson) ConvertObjectMembersMapToOrderedSlice(attrMap map[string]ObjectMembersInfo) (attrMapSlice []ObjectMemberAndInfo) {

	for i := 1; i < len(attrMap)+1; i++ {
		for attr, info := range attrMap {
			if i == info.Position {
				newMember := ObjectMemberAndInfo{
					ObjectMembersInfo: ObjectMembersInfo{
						VarType:     info.VarType,
						IsKey:       info.IsKey,
						IsArray:     info.IsArray,
						Description: info.Description,
						DefaultVal:  info.DefaultVal,
						Position:    info.Position,
					},
					MemberName: attr,
				}
				attrMapSlice = append(attrMapSlice, newMember)
			}
		}
	}
	return
}
func (obj *ObjectInfoJson) WriteLicenseInfo(fd *os.File) {
	var lines []string
	lines = append(lines, `
//Copyright [2016] [SnapRoute Inc]
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//	 Unless required by applicable law or agreed to in writing, software
//	 distributed under the License is distributed on an "AS IS" BASIS,
//	 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	 See the License for the specific language governing permissions and
//	 limitations under the License.
//
//   This is a auto-generated file, please do not edit!
//  _______  __       __________   ___      _______.____    __    ____  __  .___________.  ______  __    __  
// |   ____||  |     |   ____\  \ /  /     /       |\   \  /  \  /   / |  | |           | /      ||  |  |  | 
// |  |__   |  |     |  |__   \  V  /     |   (----  \   \/    \/   /  |  |  ---|  |---- |  ,---- |  |__|  | 
// |   __|  |  |     |   __|   >   <       \   \      \            /   |  |     |  |     |  |     |   __   | 
// |  |     |  ----. |  |____ /  .  \  .----)   |      \    /\    /    |  |     |  |     |   ----.|  |  |  | 
// |__|     |_______||_______/__/ \__\ |_______/        \__/  \__/     |__|     |__|      \______||__|  |__| 
//                                                                                                           
	`)
	for _, line := range lines {
		fd.WriteString(line)
	}
	fd.Sync()
}

func (obj *ObjectInfoJson) WriteDBFunctions(str *ast.StructType, attrMap map[string]ObjectMembersInfo, objMap map[string]ObjectInfoJson) {
	fileHeaderOptionalForState := ""
	dbFile, err := os.Create(obj.DbFileName)
	if err != nil {
		fmt.Println("Failed to open the file", obj.DbFileName)
		return
	}
	defer dbFile.Close()
	obj.WriteLicenseInfo(dbFile)
	attrMapSlice := obj.ConvertObjectMembersMapToOrderedSlice(attrMap)
	if strings.Contains(obj.Access, "w") || strings.Contains(obj.Access, "rw") {
		dbFile.WriteString(fileHeader)
		obj.WriteStoreObjectInDBFcn(str, dbFile, attrMapSlice, objMap)
		obj.WriteDeleteObjectFromDbFcn(str, dbFile, attrMapSlice, objMap)
		obj.WriteGetObjectFromDbFcn(str, dbFile, attrMapSlice, objMap)
		obj.WriteKeyRelatedFcns(str, dbFile, attrMapSlice, objMap)
		obj.WriteGetAllObjFromDbFcn(str, dbFile, attrMapSlice, objMap)
		obj.WriteCompareObjectsAndDiffFcn(str, dbFile, attrMapSlice, objMap)
		obj.WriteUpdateObjectInDbFcn(str, dbFile, attrMapSlice, objMap)
		obj.WriteCopyRecursiveFcn(str, dbFile)
		obj.WriteMergeDbAndConfigObjFcn(str, dbFile, attrMapSlice, objMap)
		obj.WriteMergeDbAndConfigObjForPatchUpdateFcn(str, dbFile, attrMapSlice, objMap)
		obj.WriteGetBulkObjFromDbFcn(str, dbFile, attrMapSlice, objMap)
	} else {
		if obj.UsesStateDB {
			for _, attrInfo := range attrMap {
				if attrInfo.IsArray == true {
					if _, ok := goBasicTypesMap[attrInfo.VarType]; !ok {
						fileHeaderOptionalForState = fileHeaderOptionalForState +
							`       "encoding/json"`
					}
				}
			}
		}
		dbFile.WriteString(fileHeaderForState)
		dbFile.WriteString(fileHeaderOptionalForState)
		dbFile.WriteString(endFileHeaderState)
		obj.WriteKeyRelatedFcns(str, dbFile, attrMapSlice, objMap)
		if obj.UsesStateDB {
			obj.WriteStoreObjectInDBFcn(str, dbFile, attrMapSlice, objMap)
			obj.WriteDeleteObjectFromDbFcn(str, dbFile, attrMapSlice, objMap)
			obj.WriteGetObjectFromDbFcn(str, dbFile, attrMapSlice, objMap)
			obj.WriteGetAllObjFromDbFcn(str, dbFile, attrMapSlice, objMap)
			obj.WriteGetBulkObjFromDbFcn(str, dbFile, attrMapSlice, objMap)
		}
	}
	dbFile.Sync()
}
