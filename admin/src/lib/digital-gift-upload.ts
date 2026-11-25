import {giftAPI,type GiftFile} from './digital-gifts';
type Upload={token:string;size:number;received:number;file_id?:number};
const chunkSize=8*1024*1024;
const aborted=()=>new DOMException('上传已取消','AbortError');
export async function uploadGiftFile(id:number,file:File,onProgress:(bytes:number)=>void,signal:AbortSignal):Promise<GiftFile>{
 let token:string|undefined,finished=false;
 const path=`/${id}/uploads`;
 try{
  let upload:Upload=await giftAPI<Upload>(path,'POST',{name:file.name,size:file.size},signal);
  token=upload.token;
  onProgress(upload.received);
  while(upload.received<file.size){
   if(signal.aborted)throw aborted();
   const offset=upload.received;
   let failure:Error|undefined;
   for(let attempt=0;attempt<3;attempt++){
    try{
     const response=await fetch(`/api/admin/gifts${path}/${upload.token}`,{method:'PUT',credentials:'include',cache:'no-store',signal,headers:{'Content-Type':'application/octet-stream','Upload-Offset':String(offset)},body:file.slice(offset,Math.min(file.size,offset+chunkSize))});
     if(!response.ok)throw Error('文件片段上传失败，请重试');
     const result=await response.json();if(result.code!==200)throw Error(result.message||'文件片段上传失败');
     upload=result.data;failure=undefined;break;
    }catch(error){
     if(signal.aborted)throw aborted();
     failure=error instanceof Error?error:Error('上传失败');
     // A response can be lost after the server accepted a chunk.
     try { const status:Upload=await giftAPI<Upload>(`${path}/${upload.token}`,'GET',undefined,signal);
     if(status.received>offset){upload=status;failure=undefined;break}
     } catch { if(signal.aborted)throw aborted(); }
     if(attempt<2)await new Promise(resolve=>setTimeout(resolve,800));
    }
   }
   if(failure)throw failure;
   if(upload.received<=offset||upload.received>file.size)throw Error('上传进度异常，请重新选择文件');
   onProgress(upload.received);
  }
  let result:GiftFile;
  try{result=await giftAPI<GiftFile>(`${path}/${upload.token}/complete`,'POST',{},signal)}
  catch{if(signal.aborted)throw aborted();result=await giftAPI<GiftFile>(`${path}/${upload.token}/complete`,'POST',{},signal)}
  finished=true;return result;
 }finally{
  if(token&&!finished)await giftAPI(`${path}/${token}`,'DELETE').catch(()=>{});
 }
}
