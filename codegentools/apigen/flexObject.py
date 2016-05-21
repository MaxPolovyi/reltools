import json
import operator
def isNumericAttr (attrInfo) :
    if attrInfo['isArray'] == 'False':
        return attrInfo['type'] in ["int32", "uint32", "uint8", "uint16", "int16"]
    else:
        return False

def isBoolean(attrType) :
    return attrType in ["bool"]

def boolFromString (val) :
    if val == 'false':
        return False
    else:
        return True 

class FlexObject(object) :
    TAB = "    " 
    def __init__ (self,     # Yours truly
                  name,     # Object Name 
                  access,   # Access r/w
                  multiplicity, # UML notation *=many 1=1
                  attrFile): # Location of the attributes description
        self.name = str(name)
        self.access = access
        self.attrFile = attrFile
        self.multiplicity = multiplicity
        self.attrDict = {}
        self.attrList = None
        
        with open(attrFile) as fileHdl:
            attrDict = json.load(fileHdl)
            attrList = [None] *len(attrDict)
            keysList = []
            dfltAttrList = []
            for attrName, tmpInfo in attrDict.iteritems():
                tmpDict = {}
                for k,v in tmpInfo.iteritems():
                    tmpDict[str(k)] = str(v)

                self.attrDict[str(attrName)] = tmpDict
                if tmpDict['isKey'] == 'True':
                    keysList.append((attrName, self.attrDict[str(attrName)]))
                elif tmpDict['default'] != '':
                    dfltAttrList.append((attrName, self.attrDict[str(attrName)]))
                else:
                    attrList[tmpInfo['position'] -1] = (attrName, self.attrDict[str(attrName)])

            self.attrList =  keysList + [x for x in attrList if x!= None] + dfltAttrList 

                
    def createGetByIdMethod (self, fileHdl):
        tabs = self.TAB
        #lines = [ "\n"+ tabs + "@processReturnCode"]
        lines = []
        lines.append("\n"+ tabs + "def get" + self.name + "ById(self, objectId ):\n")
        tabs = tabs + self.TAB
        if self.name.endswith('State'):
            objName = self.name[:-5]
        else:
            objName = self.name
        lines.append (tabs + "reqUrl =  self.stateUrlBase+" +"\'%s\'" %(objName))
        lines[-1] = lines[-1] + "+\"/%s\"%(objectId)\n"
        lines.append(tabs + "r = requests.get(reqUrl, data=None, headers=headers) \n")
        lines.append(tabs + "return r\n")                                                                                  
        fileHdl.writelines(lines)

    def createGetMethod (self, fileHdl):
        tabs = self.TAB
        #lines = [ "\n"+ tabs + "@processReturnCode"]
        lines = []
        lines.append("\n"+ tabs + "def get" + self.name + "(self,")
        tabs = tabs + self.TAB
        spaces = ' ' * (len(lines[-1])  - len("self, "))
        objLines = [tabs + "obj =  { \n"]
        argStr = ''
        for (attr, attrInfo) in self.attrList:
            if attrInfo['isKey'] == 'True':
                argStr = "\n" + spaces + "%s," %(attr)
                assignmentStr = "%s" %(attr)

                if isNumericAttr(attrInfo):
                    #argStr = "\n" + spaces + "%s=%d," %(attr,int(attrInfo['default'].lstrip()))
                    assignmentStr = "int(%s)" %(attr)
                elif isBoolean(attrInfo['type']):
                    #argStr = "\n" + spaces + "%s=%s," %(attr, boolFromString(attrInfo['default'].lstrip()))
                    assignmentStr = "True if %s else False" %(attr)

                lines.append(argStr)
                objLines.append(tabs+tabs + "\'%s\' : %s,\n" %(attr, assignmentStr))


        lines[-1] = lines[-1][0:lines[-1].find(',')]
        lines.append("):\n")
        objLines.append(tabs + tabs+"}\n")
        lines = lines + objLines
        if self.name.endswith('State'):
            objName = self.name[:-5]
        else:
            objName = self.name
        lines.append (tabs + "reqUrl =  self.cfgUrlBase+" +"\'%s\'\n" %(objName))
        lines.append(tabs + "r = requests.get(reqUrl, data=json.dumps(obj), headers=headers) \n")
        lines.append(tabs + "return r\n")                                                                                  
        fileHdl.writelines(lines)

    def createGetAllMethod (self, fileHdl, urlPath):
        tabs = self.TAB
        lines = [ "\n"+ tabs + "def getAll" + self.name+"s" + "(self):\n"]
        tabs = tabs + self.TAB
        if 'r' in self.access:
            if self.name.endswith('State'):
                objName = self.name[:-5]
            else:
                objName = self.name
        else:
            objName = self.name
        lines.append (tabs + "return self.getObjects( \'%s\', %s)\n\n" %(objName, urlPath))
        fileHdl.writelines(lines)

    def writeAllMethods (self, fileHdl):
        self.createGetMethod(fileHdl)
        self.createGetByIdMethod(fileHdl)
        self.createGetAllMethod(fileHdl, 'self.stateUrlBase')

