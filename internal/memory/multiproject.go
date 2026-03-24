package memory

import ("encoding/json";"fmt";"os";"path/filepath";"strings";"time";"github.com/arbazkhan971/memorx/internal/storage";"github.com/google/uuid")

type GlobalSearchResult struct{ProjectName,ProjectPath,Type,Content,CreatedAt string}
type LinkedProject struct{ID,ProjectPath,ProjectName,Relationship,CreatedAt string}
type PatternInfo struct{Pattern string;Projects []string;Count int}
type TemplateData struct{Name string `json:"name"`;Decisions []string `json:"decisions"`;Facts []TemplateFact `json:"facts"`;PlanSteps []string `json:"plan_steps"`;CreatedAt string `json:"created_at"`}
type TemplateFact struct{Subject string `json:"subject"`;Predicate string `json:"predicate"`;Object string `json:"object"`}

func findMemoryDBs() []string {
	home,_:=os.UserHomeDir();if home==""{return nil};var dbs []string
	for _,d:=range []string{"Projects","projects","dev","src","code","workspace","repos","Code","Developer"}{entries,err:=os.ReadDir(filepath.Join(home,d));if err!=nil{continue};for _,e:=range entries{if e.IsDir(){p:=filepath.Join(home,d,e.Name(),".memory","memory.db");if _,err:=os.Stat(p);err==nil{dbs=append(dbs,p)}}}};return dbs
}
func GlobalSearch(query string) ([]GlobalSearchResult,error) {
	dbPaths:=findMemoryDBs();if len(dbPaths)==0{return nil,nil};tokens:=strings.Fields(query);for i,t:=range tokens{tokens[i]="\""+strings.ReplaceAll(t,"\"","")+"\""};san:=strings.Join(tokens," ")
	var results []GlobalSearchResult;for _,dbPath:=range dbPaths{pp:=filepath.Dir(filepath.Dir(dbPath));pn:=filepath.Base(pp);db,err:=storage.NewDB(dbPath);if err!=nil{continue}
	for _,q:=range []struct{query,typ string}{{`SELECT n.content,n.created_at FROM notes_fts fts JOIN notes n ON n.rowid=fts.rowid WHERE notes_fts MATCH ? LIMIT 5`,"note"},{`SELECT fa.subject||' '||fa.predicate||' '||fa.object,fa.valid_at FROM facts_fts fts JOIN facts fa ON fa.rowid=fts.rowid WHERE facts_fts MATCH ? AND fa.invalid_at IS NULL LIMIT 5`,"fact"},{`SELECT c.message,c.committed_at FROM commits_fts fts JOIN commits c ON c.rowid=fts.rowid WHERE commits_fts MATCH ? LIMIT 5`,"commit"}}{rows,err:=db.Reader().Query(q.query,san);if err!=nil{continue};for rows.Next(){var c,t string;if rows.Scan(&c,&t)==nil{results=append(results,GlobalSearchResult{ProjectName:pn,ProjectPath:pp,Type:q.typ,Content:c,CreatedAt:t})}};rows.Close()};db.Close()};return results,nil
}
func FormatGlobalSearch(results []GlobalSearchResult) string {
	if len(results)==0{return "No results found across projects."};var b strings.Builder;fmt.Fprintf(&b,"# Global search results (%d found)\n\n",len(results));grouped:=make(map[string][]GlobalSearchResult);var order []string;for _,r:=range results{if _,ok:=grouped[r.ProjectName];!ok{order=append(order,r.ProjectName)};grouped[r.ProjectName]=append(grouped[r.ProjectName],r)};for _,proj:=range order{fmt.Fprintf(&b,"## %s\n",proj);for _,item:=range grouped[proj]{c:=strings.ReplaceAll(item.Content,"\n"," ");if len(c)>100{c=c[:100]+"..."};fmt.Fprintf(&b,"- [%s] %s\n",item.Type,c)};b.WriteString("\n")};return b.String()
}
func DetectPatterns() ([]PatternInfo,error) {
	dbPaths:=findMemoryDBs();if len(dbPaths)==0{return nil,nil};fm:=make(map[string]map[string][]string)
	for _,dbPath:=range dbPaths{pn:=filepath.Base(filepath.Dir(filepath.Dir(dbPath)));db,err:=storage.NewDB(dbPath);if err!=nil{continue};rows,err:=db.Reader().Query(`SELECT predicate,object FROM facts WHERE invalid_at IS NULL`);if err==nil{for rows.Next(){var p,o string;if rows.Scan(&p,&o)==nil{if fm[p]==nil{fm[p]=make(map[string][]string)};found:=false;for _,x:=range fm[p][o]{if x==pn{found=true;break}};if !found{fm[p][o]=append(fm[p][o],pn)}}};rows.Close()};db.Close()}
	var patterns []PatternInfo;for pred,objs:=range fm{for obj,projs:=range objs{if len(projs)>=2{patterns=append(patterns,PatternInfo{Pattern:pred+" "+obj,Projects:projs,Count:len(projs)})}}};for i:=1;i<len(patterns);i++{for j:=i;j>0&&patterns[j].Count>patterns[j-1].Count;j--{patterns[j],patterns[j-1]=patterns[j-1],patterns[j]}};return patterns,nil
}
func FormatPatterns(patterns []PatternInfo) string {
	if len(patterns)==0{return "No cross-project patterns detected. Patterns emerge when 2+ projects share facts."};var b strings.Builder;b.WriteString("# Cross-project patterns\n\n");for _,p:=range patterns{fmt.Fprintf(&b,"- **%s** (%d projects: %s)\n",p.Pattern,p.Count,strings.Join(p.Projects,", "))};return b.String()
}
func (s *Store) SaveTemplate(featureID,templateName string) error {
	home,err:=os.UserHomeDir();if err!=nil{return err};dir:=filepath.Join(home,".memorx","templates");os.MkdirAll(dir,0755);td:=TemplateData{Name:templateName,CreatedAt:time.Now().UTC().Format(time.DateTime)}
	notes,_:=s.ListNotes(featureID,"decision",50);for _,n:=range notes{td.Decisions=append(td.Decisions,n.Content)};facts,_:=s.GetActiveFacts(featureID);for _,f:=range facts{td.Facts=append(td.Facts,TemplateFact{Subject:f.Subject,Predicate:f.Predicate,Object:f.Object})}
	var planID string;if s.db.Reader().QueryRow(`SELECT id FROM plans WHERE feature_id=? AND status='active' ORDER BY created_at DESC LIMIT 1`,featureID).Scan(&planID)==nil{if rows,err:=s.db.Reader().Query(`SELECT title FROM plan_steps WHERE plan_id=? ORDER BY step_number`,planID);err==nil{for rows.Next(){var t string;if rows.Scan(&t)==nil{td.PlanSteps=append(td.PlanSteps,t)}};rows.Close()}}
	data,_:=json.MarshalIndent(td,"","  ");return os.WriteFile(filepath.Join(dir,templateName+".json"),data,0644)
}
func (s *Store) ApplyTemplate(featureID,sessionID,templateName string) (*TemplateData,error) {
	home,err:=os.UserHomeDir();if err!=nil{return nil,err};data,err:=os.ReadFile(filepath.Join(home,".memorx","templates",templateName+".json"));if err!=nil{return nil,fmt.Errorf("read template %q: %w",templateName,err)};var td TemplateData;if err:=json.Unmarshal(data,&td);err!=nil{return nil,err};for _,d:=range td.Decisions{s.CreateNote(featureID,sessionID,d,"decision")};for _,f:=range td.Facts{s.CreateFact(featureID,sessionID,f.Subject,f.Predicate,f.Object)};return &td,nil
}
func (s *Store) LinkProject(projectPath,relationship string) (*LinkedProject,error) {
	if relationship==""{relationship="related"};info,err:=os.Stat(projectPath);if err!=nil{return nil,fmt.Errorf("project path %q not found: %w",projectPath,err)};if !info.IsDir(){return nil,fmt.Errorf("project path %q is not a directory",projectPath)}
	id:=uuid.New().String();now:=time.Now().UTC().Format(time.DateTime);pn:=filepath.Base(projectPath);if _,err:=s.db.Writer().Exec(`INSERT OR REPLACE INTO linked_projects (id,project_path,project_name,relationship,created_at) VALUES (?,?,?,?,?)`,id,projectPath,pn,relationship,now);err!=nil{return nil,fmt.Errorf("link project: %w",err)};return &LinkedProject{ID:id,ProjectPath:projectPath,ProjectName:pn,Relationship:relationship,CreatedAt:now},nil
}
func (s *Store) ListLinkedProjects() ([]LinkedProject,error) {
	rows,err:=s.db.Reader().Query(`SELECT id,project_path,project_name,relationship,created_at FROM linked_projects ORDER BY created_at DESC`);if err!=nil{return nil,err};defer rows.Close();var out []LinkedProject;for rows.Next(){var lp LinkedProject;if rows.Scan(&lp.ID,&lp.ProjectPath,&lp.ProjectName,&lp.Relationship,&lp.CreatedAt)==nil{out=append(out,lp)}};return out,rows.Err()
}
