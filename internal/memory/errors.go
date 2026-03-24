package memory

import ("fmt";"strings";"time";"github.com/arbazkhan971/memorx/internal/search";"github.com/google/uuid")

type ErrorEntry struct {ID,FeatureID,SessionID string;ErrorMessage,FilePath string;LineNumber int;Cause,Resolution string;Resolved bool;CreatedAt string}
type TestResult struct {ID,FeatureID,SessionID string;TestName string;Passed bool;ErrorMessage string;CreatedAt string}

func (s *Store) LogError(featureID,sessionID,message,filePath,cause,resolution string) (*ErrorEntry,error) {
	id:=uuid.New().String();now:=time.Now().UTC().Format(time.DateTime);w:=s.db.Writer()
	resolved:=0;if resolution!="" {resolved=1}
	if _,err:=w.Exec(`INSERT INTO error_log (id,feature_id,session_id,error_message,file_path,cause,resolution,resolved,created_at) VALUES (?,?,?,?,?,?,?,?,?)`,id,featureID,nullIfEmpty(sessionID),message,nullIfEmpty(filePath),nullIfEmpty(cause),nullIfEmpty(resolution),resolved,now);err!=nil{return nil,fmt.Errorf("log error: %w",err)}
	var rowID int64;if err:=w.QueryRow(`SELECT rowid FROM error_log WHERE id=?`,id).Scan(&rowID);err!=nil{return nil,fmt.Errorf("get error rowid: %w",err)}
	if _,err:=w.Exec(`INSERT INTO error_log_fts(rowid,error_message,cause,resolution) VALUES (?,?,?,?)`,rowID,message,cause,resolution);err!=nil{return nil,fmt.Errorf("sync error to fts: %w",err)}
	return &ErrorEntry{ID:id,FeatureID:featureID,SessionID:sessionID,ErrorMessage:message,FilePath:filePath,Cause:cause,Resolution:resolution,Resolved:resolved==1,CreatedAt:now},nil
}
func (s *Store) SearchErrors(query string) ([]ErrorEntry,error) {
	tokens:=strings.Fields(query);for i,t:=range tokens{tokens[i]="\""+strings.ReplaceAll(t,"\"","")+"\""};san:=strings.Join(tokens," ")
	rows,err:=s.db.Reader().Query(`SELECT e.id,e.feature_id,COALESCE(e.session_id,''),e.error_message,COALESCE(e.file_path,''),COALESCE(e.line_number,0),COALESCE(e.cause,''),COALESCE(e.resolution,''),e.resolved,e.created_at FROM error_log_fts fts JOIN error_log e ON e.rowid=fts.rowid WHERE error_log_fts MATCH ? ORDER BY e.created_at DESC LIMIT 20`,san)
	if err!=nil{return nil,fmt.Errorf("search errors: %w",err)};defer rows.Close()
	var out []ErrorEntry;for rows.Next(){var e ErrorEntry;var ri int;if rows.Scan(&e.ID,&e.FeatureID,&e.SessionID,&e.ErrorMessage,&e.FilePath,&e.LineNumber,&e.Cause,&e.Resolution,&ri,&e.CreatedAt)==nil{e.Resolved=ri==1;out=append(out,e)}};return out,rows.Err()
}
func FormatErrorSearch(errors []ErrorEntry) string {
	if len(errors)==0{return "No matching errors found."};var b strings.Builder;fmt.Fprintf(&b,"# Error search results (%d found)\n\n",len(errors))
	for _,e:=range errors{st:="UNRESOLVED";if e.Resolved{st="RESOLVED"};fmt.Fprintf(&b,"## [%s] %s\n",st,errTrunc(e.ErrorMessage,100));if e.FilePath!=""{fmt.Fprintf(&b,"- File: %s\n",e.FilePath)};if e.Cause!=""{fmt.Fprintf(&b,"- Cause: %s\n",e.Cause)};if e.Resolution!=""{fmt.Fprintf(&b,"- Resolution: %s\n",e.Resolution)};fmt.Fprintf(&b,"- Logged: %s\n\n",e.CreatedAt)};return b.String()
}
type DebugContext struct {Decisions,Facts,Files,Commits []RelatedItem;Errors []ErrorEntry;TestResults []TestResult}
func (s *Store) GetDebugContext(engine *search.Engine,topic string) (*DebugContext,error) {
	dc:=&DebugContext{};if r,err:=s.FindRelated(engine,topic,2);err==nil&&r!=nil{dc.Decisions,dc.Facts,dc.Files,dc.Commits=r.Decisions,r.Facts,r.Files,r.Commits}
	p:="%"+topic+"%"
	if rows,err:=s.db.Reader().Query(`SELECT id,feature_id,COALESCE(session_id,''),error_message,COALESCE(file_path,''),COALESCE(line_number,0),COALESCE(cause,''),COALESCE(resolution,''),resolved,created_at FROM error_log WHERE error_message LIKE ? OR file_path LIKE ? OR cause LIKE ? ORDER BY created_at DESC LIMIT 10`,p,p,p);err==nil{defer rows.Close();for rows.Next(){var e ErrorEntry;var ri int;if rows.Scan(&e.ID,&e.FeatureID,&e.SessionID,&e.ErrorMessage,&e.FilePath,&e.LineNumber,&e.Cause,&e.Resolution,&ri,&e.CreatedAt)==nil{e.Resolved=ri==1;dc.Errors=append(dc.Errors,e)}}}
	if rows,err:=s.db.Reader().Query(`SELECT id,feature_id,COALESCE(session_id,''),test_name,passed,COALESCE(error_message,''),created_at FROM test_results WHERE test_name LIKE ? OR error_message LIKE ? ORDER BY created_at DESC LIMIT 10`,p,p);err==nil{defer rows.Close();for rows.Next(){var tr TestResult;var pi int;if rows.Scan(&tr.ID,&tr.FeatureID,&tr.SessionID,&tr.TestName,&pi,&tr.ErrorMessage,&tr.CreatedAt)==nil{tr.Passed=pi==1;dc.TestResults=append(dc.TestResults,tr)}}}
	return dc,nil
}
func FormatDebugContext(dc *DebugContext,topic string) string {
	var b strings.Builder;fmt.Fprintf(&b,"# Debug context: %s\n\n",topic)
	sect:=func(t string,items []RelatedItem){if len(items)==0{return};fmt.Fprintf(&b,"## %s\n",t);for _,i:=range items{c:=strings.ReplaceAll(i.Content,"\n"," ");if len(c)>120{c=c[:120]+"..."};fmt.Fprintf(&b,"- %s\n",c)};b.WriteString("\n")}
	sect("Related decisions",dc.Decisions);sect("Related facts",dc.Facts);sect("Related files",dc.Files);sect("Related commits",dc.Commits)
	if len(dc.Errors)>0{b.WriteString("## Known errors\n");for _,e:=range dc.Errors{st:="UNRESOLVED";if e.Resolved{st="RESOLVED"};fmt.Fprintf(&b,"- [%s] %s\n",st,errTrunc(e.ErrorMessage,80))};b.WriteString("\n")}
	if len(dc.TestResults)>0{b.WriteString("## Test results\n");for _,tr:=range dc.TestResults{st:="PASS";if !tr.Passed{st="FAIL"};fmt.Fprintf(&b,"- [%s] %s @ %s\n",st,tr.TestName,tr.CreatedAt)};b.WriteString("\n")}
	if b.Len()<=len(fmt.Sprintf("# Debug context: %s\n\n",topic)){return fmt.Sprintf("No debug context found for topic %q.",topic)};return b.String()
}
func (s *Store) RecordTestResult(featureID,sessionID,testName string,passed bool,errorMessage string) (*TestResult,error) {
	id:=uuid.New().String();now:=time.Now().UTC().Format(time.DateTime);pi:=0;if passed{pi=1}
	if _,err:=s.db.Writer().Exec(`INSERT INTO test_results (id,feature_id,session_id,test_name,passed,error_message,created_at) VALUES (?,?,?,?,?,?,?)`,id,featureID,nullIfEmpty(sessionID),testName,pi,nullIfEmpty(errorMessage),now);err!=nil{return nil,fmt.Errorf("record test result: %w",err)}
	return &TestResult{ID:id,FeatureID:featureID,SessionID:sessionID,TestName:testName,Passed:passed,ErrorMessage:errorMessage,CreatedAt:now},nil
}
func (s *Store) GetTestHistory(testName string,limit int) ([]TestResult,error) {
	if limit<=0{limit=10};rows,err:=s.db.Reader().Query(`SELECT id,feature_id,COALESCE(session_id,''),test_name,passed,COALESCE(error_message,''),created_at FROM test_results WHERE test_name=? ORDER BY created_at DESC LIMIT ?`,testName,limit)
	if err!=nil{return nil,err};defer rows.Close();var out []TestResult;for rows.Next(){var tr TestResult;var pi int;if rows.Scan(&tr.ID,&tr.FeatureID,&tr.SessionID,&tr.TestName,&pi,&tr.ErrorMessage,&tr.CreatedAt)==nil{tr.Passed=pi==1;out=append(out,tr)}};return out,rows.Err()
}
func FormatTestMemory(current *TestResult,history []TestResult) string {
	var b strings.Builder;st:="PASS";if !current.Passed{st="FAIL"};fmt.Fprintf(&b,"# Test recorded: %s [%s]\n\n",current.TestName,st)
	if current.ErrorMessage!=""{fmt.Fprintf(&b,"Error: %s\n\n",current.ErrorMessage)}
	if len(history)>1{pc,fc:=0,0;for _,tr:=range history{if tr.Passed{pc++}else{fc++}};fmt.Fprintf(&b,"## History (%d runs)\n- Pass: %d, Fail: %d\n",len(history),pc,fc);for _,tr:=range history{s:="PASS";if !tr.Passed{s="FAIL"};fmt.Fprintf(&b,"- [%s] %s\n",s,tr.CreatedAt)}};return b.String()
}
func errTrunc(s string,n int) string {s=strings.ReplaceAll(s,"\n"," ");if len(s)<=n{return s};return s[:n]+"..."}
