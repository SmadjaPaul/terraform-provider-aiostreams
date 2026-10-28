package aiostreams

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "strings"
 "time"
)

type Client struct { BaseURL string; HTTP *http.Client }
func New(baseURL string) (*Client,error) {
 u,err:=url.Parse(baseURL); if err!=nil || u.Scheme!="https" || u.Host=="" || u.User!=nil || u.RawQuery!="" || u.Fragment!="" {return nil,errors.New("AIOStreams endpoint must be an HTTPS origin")}
 return &Client{BaseURL:strings.TrimRight(baseURL,"/"),HTTP:&http.Client{Timeout:20*time.Second}},nil
}
type response struct { Success bool `json:"success"`; Detail string `json:"detail"`; Data json.RawMessage `json:"data"` }
type Credentials struct { UUID,Password string }
type Created struct { UUID string `json:"uuid"` }
type Retrieved struct { UserData json.RawMessage `json:"userData"` }
func (c *Client) request(ctx context.Context, method string, cred *Credentials, payload any, out any) error {
 var body io.Reader
 if payload!=nil { b,err:=json.Marshal(payload);if err!=nil{return err};body=bytes.NewReader(b) }
 req,err:=http.NewRequestWithContext(ctx,method,c.BaseURL+"/api/v1/user",body); if err!=nil{return err}
 req.Header.Set("Accept","application/json")
 if payload!=nil {req.Header.Set("Content-Type","application/json")}
 if cred!=nil {req.SetBasicAuth(cred.UUID,cred.Password)}
 res,err:=c.HTTP.Do(req);if err!=nil{return fmt.Errorf("AIOStreams request failed: %w",err)}
 defer res.Body.Close()
 if res.StatusCode<200 || res.StatusCode>=300 {io.Copy(io.Discard,io.LimitReader(res.Body,8192));return fmt.Errorf("AIOStreams %s returned HTTP %d (response suppressed)",method,res.StatusCode)}
 var envelope response
 if err:=json.NewDecoder(io.LimitReader(res.Body,4<<20)).Decode(&envelope);err!=nil{return fmt.Errorf("invalid AIOStreams JSON response: %w",err)}
 if !envelope.Success {return errors.New("AIOStreams request returned unsuccessful envelope (detail suppressed)")}
 if out!=nil {if err:=json.Unmarshal(envelope.Data,out);err!=nil{return fmt.Errorf("invalid AIOStreams response data: %w",err)}}
 return nil
}
func (c *Client) Create(ctx context.Context,password string,config json.RawMessage)(Created,error){
 var out Created
 if !json.Valid(config) || len(config)==0 || config[0]!='{' {return out,errors.New("config must be a JSON object")}
 err:=c.request(ctx,http.MethodPost,nil,map[string]any{"password":password,"config":config},&out)
 if err==nil && out.UUID=="" {err=errors.New("AIOStreams did not return UUID")}
 return out,err
}
func (c *Client) Get(ctx context.Context,cred Credentials)(json.RawMessage,error){
 var out Retrieved
 err:=c.request(ctx,http.MethodGet,&cred,nil,&out)
 return out.UserData,err
}
func (c *Client) Update(ctx context.Context,cred Credentials,config json.RawMessage)error{
 if !json.Valid(config) || len(config)==0 || config[0]!='{' {return errors.New("config must be a JSON object")}
 return c.request(ctx,http.MethodPut,&cred,map[string]any{"config":config},nil)
}
