package browser

// stealth_scripts.go — JavaScript injection constants for anti-bot stealth.
// Single responsibility: define JS snippets. No Go logic.

// canvasNoiseJS injects imperceptible ±1 RGB noise into canvas operations.
// This breaks canvas fingerprint tracking — each call produces a unique hash.
const canvasNoiseJS = `
(function(){
  const origTDU = HTMLCanvasElement.prototype.toDataURL;
  const origGID = CanvasRenderingContext2D.prototype.getImageData;
  function addNoise(data){
    for(let i=0;i<data.length;i+=4){
      data[i]   = Math.max(0,Math.min(255,data[i]  +(Math.random()*3|0)-1));
      data[i+1] = Math.max(0,Math.min(255,data[i+1]+(Math.random()*3|0)-1));
      data[i+2] = Math.max(0,Math.min(255,data[i+2]+(Math.random()*3|0)-1));
    }
  }
  HTMLCanvasElement.prototype.toDataURL = function(){
    const ctx = this.getContext('2d');
    if(ctx){try{
      const d = origGID.call(ctx,0,0,this.width,this.height);
      addNoise(d.data); ctx.putImageData(d,0,0);
    }catch(e){}}
    return origTDU.apply(this,arguments);
  };
  CanvasRenderingContext2D.prototype.getImageData = function(){
    const d = origGID.apply(this,arguments);
    addNoise(d.data); return d;
  };
})();`

// blockWebRTCJS neuters RTCPeerConnection to prevent real IP leaks via ICE candidates.
const blockWebRTCJS = `
(function(){
  const dummy = function(){
    return {close:()=>{},createDataChannel:()=>({}),
      setLocalDescription:()=>Promise.resolve(),
      createOffer:()=>Promise.resolve({}),
      addEventListener:()=>{},removeEventListener:()=>{},
      onicecandidate:null,ontrack:null};
  };
  window.RTCPeerConnection = dummy;
  window.webkitRTCPeerConnection = dummy;
  window.mozRTCPeerConnection = dummy;
  if(navigator.mediaDevices){
    navigator.mediaDevices.getUserMedia = ()=>
      Promise.reject(new DOMException('NotAllowedError'));
    navigator.mediaDevices.enumerateDevices = ()=>Promise.resolve([]);
  }
})();`

// mockPluginsJS replaces the empty headless plugin list with realistic Chrome plugins
// and adds the chrome.runtime object that headless Chrome lacks.
const mockPluginsJS = `
(function(){
  function mkPlugin(n,f,d){
    return {name:n,filename:f,description:d,length:1,
      item:()=>({type:'application/pdf'}),
      namedItem:()=>({type:'application/pdf'})};
  }
  const plugins = [
    mkPlugin('Chrome PDF Plugin','internal-pdf-viewer','Portable Document Format'),
    mkPlugin('Chrome PDF Viewer','mhjfbmdgcfjbbpaeojofohoefgiehjai',''),
    mkPlugin('Native Client','internal-nacl-plugin','')
  ];
  Object.defineProperty(navigator,'plugins',{get:()=>{
    const a=Object.create(PluginArray.prototype);
    plugins.forEach((p,i)=>{a[i]=p});
    Object.defineProperty(a,'length',{get:()=>3});
    a.item = i=>plugins[i];
    a.namedItem = n=>plugins.find(p=>p.name===n);
    a.refresh = ()=>{};
    return a;
  }});
  if(!window.chrome) window.chrome={};
  if(!window.chrome.runtime) window.chrome.runtime={
    connect:()=>({onMessage:{addListener:()=>{}},postMessage:()=>{}}),
    sendMessage:()=>{},
    onMessage:{addListener:()=>{},removeListener:()=>{}},
    id:undefined
  };
  if(!window.chrome.loadTimes) window.chrome.loadTimes=()=>({});
  if(!window.chrome.csi) window.chrome.csi=()=>({});
})();`
