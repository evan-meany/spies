// Open browser DevTools and run this in two different tabs
const ws = new WebSocket("ws://localhost:8080/ws");
ws.onmessage = (e)=>console.log("<<<", e.data);
ws.onopen = ()=> {
  ws.send(JSON.stringify({type:"hello", name:"Tab-"+Math.floor(Math.random()*1000)}));
  setTimeout(()=>ws.send(JSON.stringify({type:"chat", text:"hi!"})), 500);
  setTimeout(()=>ws.send(JSON.stringify({type:"move", move:"PLAY:7H"})), 1500);
};