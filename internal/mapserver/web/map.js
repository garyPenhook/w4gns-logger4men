'use strict';
const $ = id => document.getElementById(id);
const colors={'160M':'#b69cff','80M':'#aebcff','60M':'#df9dd2','40M':'#eead79','30M':'#e7cd75','20M':'#70dfc4','17M':'#73cce5','15M':'#7eafff','12M':'#c4df7e','10M':'#f192a4','6M':'#dfbcfa'};
let reports=new Map(), state={}, world=[], usStates=[], visible=[], selected='', inspectorPoint='', page=0, hits=[], scale=1, pan=[0,0], drag=null, online=false, lastPacket=0;
const canvas=$('map'), ctx=canvas.getContext('2d');
const key=r=>r.DXCall+' / '+r.Band;
for(const [band,color] of Object.entries(colors)) {
  const option=document.createElement('option'); option.value=band; option.textContent=band; $('band').append(option);
  const item=document.createElement('span'), dot=document.createElement('span'); dot.className='swatch'; dot.style.backgroundColor=color; item.append(dot,band); $('legend').append(item);
}
// Preferences contain only display controls, never cluster reports or credentials.
const viewControls=['band','age','filters','paths','origin'];
try {const prefs=JSON.parse(localStorage.getItem('w4gns-map-view')||'{}'); for(const id of viewControls) if([...$(id).options].some(o=>o.value===prefs[id])) $(id).value=prefs[id];}catch{}
function remember(){try{localStorage.setItem('w4gns-map-view',JSON.stringify(Object.fromEntries(viewControls.map(id=>[id,$(id).value]))));}catch{}}
// isUSA reports whether a resolved location's country is the USA, per the
// bundled DXCC country-reference data (see internal/geo.Location.Country) —
// used by the Location filter to separate domestic from DX activity.
function isUSA(loc){return loc?.Country==='United States';}
function size(){const box=canvas.getBoundingClientRect(),dpr=window.devicePixelRatio||1; if(canvas.width!==Math.round(box.width*dpr)||canvas.height!==Math.round(box.height*dpr)){canvas.width=Math.round(box.width*dpr);canvas.height=Math.round(box.height*dpr);}ctx.setTransform(dpr,0,0,dpr,0,0);return [box.width,box.height];}
function project(lon,lat,w,h){const unit=Math.min(w/360,h/180)*scale;return [w/2+lon*unit+pan[0],h/2-lat*unit+pan[1]];}
function strokeRing(ring,w,h,close=false){ctx.beginPath(); ring.forEach(([lon,lat],i)=>{const [x,y]=project(lon,lat,w,h); if(i===0)ctx.moveTo(x,y);else ctx.lineTo(x,y);});if(close)ctx.closePath();}
// Spherical interpolation follows the short great-circle route. Degenerate
// antipodal pairs have no unique route and deliberately draw no path.
function greatCircle(a,b){const rad=Math.PI/180, vec=p=>{const lat=p.Latitude*rad,lon=p.Longitude*rad;return [Math.cos(lat)*Math.cos(lon),Math.cos(lat)*Math.sin(lon),Math.sin(lat)];};const u=vec(a),v=vec(b),omega=Math.acos(Math.max(-1,Math.min(1,u.reduce((s,x,i)=>s+x*v[i],0))));if(omega<1e-8||Math.PI-omega<1e-6)return [];const points=[];for(let i=0;i<=80;i++){const t=i/80,A=Math.sin((1-t)*omega)/Math.sin(omega),B=Math.sin(t*omega)/Math.sin(omega),p=u.map((x,j)=>A*x+B*v[j]);points.push([Math.atan2(p[1],p[0])/rad,Math.atan2(p[2],Math.hypot(p[0],p[1]))/rad]);}return points;}
function drawPath(a,b,w,h,color){const points=greatCircle(a,b);ctx.strokeStyle=color;ctx.lineWidth=1;ctx.setLineDash([4,5]);ctx.beginPath();let last=null;for(const p of points){const [x,y]=project(...p,w,h);if(!last||Math.abs(p[0]-last[0])>180)ctx.moveTo(x,y);else ctx.lineTo(x,y);last=p;}ctx.stroke();ctx.setLineDash([]);}
function draw(){const [w,h]=size();ctx.clearRect(0,0,w,h);ctx.fillStyle='#0b1724';ctx.fillRect(0,0,w,h);ctx.strokeStyle='#192e40';ctx.lineWidth=.6;for(let lon=-180;lon<=180;lon+=30){strokeRing([[lon,-90],[lon,90]],w,h);ctx.stroke();}for(let lat=-60;lat<=60;lat+=30){strokeRing([[-180,lat],[180,lat]],w,h);ctx.stroke();}
  ctx.fillStyle='#20394a';ctx.strokeStyle='#3b5667';ctx.lineWidth=.55;
  for(const polygon of world){ctx.beginPath();for(const ring of polygon){ring.forEach(([lon,lat],i)=>{const [x,y]=project(lon,lat,w,h);if(i===0)ctx.moveTo(x,y);else ctx.lineTo(x,y);});ctx.closePath();}ctx.fill('evenodd');ctx.stroke();}
  // US state outlines: cartographic context only, drawn under everything
  // else (paths/markers) — darker than the land fill so they read as a
  // visible seam rather than blending into it.
  ctx.strokeStyle='#0a141d';ctx.lineWidth=.6;
  for(const polygon of usStates)for(const ring of polygon){ctx.beginPath();ring.forEach(([lon,lat],i)=>{const [x,y]=project(lon,lat,w,h);if(i===0)ctx.moveTo(x,y);else ctx.lineTo(x,y);});ctx.closePath();ctx.stroke();}
  const pathMode=$('paths').value;let pathCount=0;const seenPaths=new Set();
  for(const r of visible){if(pathMode==='off'||(pathMode==='selected'&&key(r)!==selected)||!r.DXLocation||!r.SpotterLocation)continue;const k=key(r)+' '+r.SpotterCall;if(seenPaths.has(k))continue;seenPaths.add(k);if(pathCount++>=500)break;drawPath(r.SpotterLocation,r.DXLocation,w,h,colors[r.Band]+'88');const [x,y]=project(r.SpotterLocation.Longitude,r.SpotterLocation.Latitude,w,h);ctx.strokeStyle=colors[r.Band];ctx.strokeRect(x-3,y-3,6,6);}
  const groups=new Map();for(const r of visible){if(!r.DXLocation)continue;const loc=r.DXLocation,k=loc.Longitude+','+loc.Latitude;if(!groups.has(k))groups.set(k,[]);groups.get(k).push(r);}
  hits=[];for(const group of groups.values()){const r=group[0],loc=r.DXLocation,[x,y]=project(loc.Longitude,loc.Latitude,w,h);if(x<0||x>w||y<0||y>h)continue;const calls=new Set(group.map(key)),spotters=new Set(group.map(v=>v.SpotterCall)),radius=Math.min(11,4+Math.log2(spotters.size+1)),active=group.some(v=>key(v)===selected);ctx.globalAlpha=Math.max(.3,1-(Date.now()-Date.parse(r.ReceivedAtUTC))/(Number($('age').value)*60000)*.6);ctx.beginPath();ctx.arc(x,y,radius,0,Math.PI*2);ctx.fillStyle=colors[r.Band]||'#fff';ctx.fill();ctx.globalAlpha=1;ctx.strokeStyle=active?'#ffffff':'#0b1724';ctx.lineWidth=active?2:1;ctx.stroke();if(calls.size>1){ctx.font='bold 9px system-ui';ctx.fillStyle='#0b1724';ctx.textAlign='center';ctx.fillText(calls.size,x,y+3);}if(active){ctx.fillStyle='#e5edf5';ctx.textAlign='left';ctx.font='12px system-ui';ctx.fillText(selected,x+radius+5,y-8);}hits.push({x,y,radius,group});}
  if(state.Home){const [x,y]=project(state.Home.Longitude,state.Home.Latitude,w,h);ctx.font='23px system-ui';ctx.textAlign='center';ctx.fillStyle='#fff';ctx.fillText('★',x,y+7);ctx.font='11px system-ui';ctx.fillText(state.Callsign||'Home',x,y-13);}
  $('mapnotice').textContent=!world.length?'World geography unavailable':pathCount>500?'Showing the first 500 paths; select a station to focus':!visible.length?'No reports match this view':'';
}
function locationText(p){return p?(p.Source===3?(p.Country?p.Country+' · approximate':'Approximate country/prefix reference'):p.Source===4?'QRZ profile location':p.Source===5?'POTA park location':p.Locator?'Grid center · '+p.Locator:'Configured location'):'Unknown location';}
function distance(a,b){const rad=Math.PI/180,p=a.Latitude*rad,q=b.Latitude*rad,d=(b.Longitude-a.Longitude)*rad;const arc=Math.acos(Math.max(-1,Math.min(1,Math.sin(p)*Math.sin(q)+Math.cos(p)*Math.cos(q)*Math.cos(d))));const bearing=(Math.atan2(Math.sin(d)*Math.cos(q),Math.cos(p)*Math.sin(q)-Math.sin(p)*Math.cos(q)*Math.cos(d))/rad+360)%360;return Math.round(arc*6371).toLocaleString()+' km · '+(arc<1e-8?'bearing undefined':Math.round(bearing)+'° true');}
function select(k){selected=k;inspectorPoint='';render();}
function details(group=null){if(group?.length)inspectorPoint=group[0].DXLocation.Longitude+','+group[0].DXLocation.Latitude;if(inspectorPoint)group=visible.filter(r=>r.DXLocation&&r.DXLocation.Longitude+','+r.DXLocation.Latitude===inspectorPoint);const target=$('details');target.replaceChildren();if(group?.length){$('selected').textContent='Stations at this point';for(const k of new Set(group.map(key))){const button=document.createElement('button');button.textContent=k;button.onclick=()=>select(k);target.append(button);}return;}
 const matching=visible.filter(r=>key(r)===selected),r=matching[0];$('selected').textContent=r?r.DXCall:'Explore the bands';if(!r){const p=document.createElement('p');p.textContent=selected?'The selected station is outside the current view.':'Select a marker or a report to inspect a station.';target.append(p);return;}
 const dl=document.createElement('dl');const fields=[['Frequency / band',(r.FrequencyHz/1e6).toFixed(4)+' MHz · '+r.Band],['DX position',locationText(r.DXLocation)],['Latest report',new Date(r.ReceivedAtUTC).toISOString().slice(11,19)+' UTC'],['Distinct reporting stations',new Set(matching.map(v=>v.SpotterCall)).size],['Latest spotter',r.SpotterCall],['Spotter position',locationText(r.SpotterLocation)],['Comment',r.Comment||'—']];if(state.Home&&r.DXLocation)fields.push(['From your station — not a reception report',distance(state.Home,r.DXLocation)]);for(const [name,value] of fields){const dt=document.createElement('dt'),dd=document.createElement('dd');dt.textContent=name;dd.textContent=value;dl.append(dt,dd);}target.append(dl);
}
// passesCommonFilters applies every render() filter except the band
// dropdown itself, so busiestBand (which tallies activity per band) reflects
// what's currently hot regardless of which single band the operator has
// selected to look at.
function passesCommonFilters(r,now,age,search,origin){return now-Date.parse(r.ReceivedAtUTC)<=age&&($('filters').value!=='follow'||r.MatchesLogger)&&(origin==='all'||isUSA(r.DXLocation)===(origin==='usa'))&&(!search||r.DXCall.includes(search)||r.SpotterCall.includes(search));}
// busiestBand tallies distinct DX-call/band combinations (same uniqueness
// the station count below uses) across every band passing the common
// filters, live off the current report set — a snapshot of what's hot right
// now, not a prediction, so it's labeled as subject to change as spots age
// out or a new band picks up.
function busiestBand(now,age,search,origin){const counts=new Map();for(const r of reports.values()){if(!passesCommonFilters(r,now,age,search,origin))continue;if(!counts.has(r.Band))counts.set(r.Band,new Set());counts.get(r.Band).add(key(r));}let band=null,count=0;for(const [b,set] of counts)if(set.size>count){band=b;count=set.size;}return band?{band,count}:null;}
function render(){const active=document.activeElement,focusRoot=active?.closest('#rows,#details')?.id,focusText=active?.textContent;const now=Date.now(),age=Number($('age').value)*60000,search=$('search').value.trim().toUpperCase(),origin=$('origin').value;visible=[...reports.values()].filter(r=>passesCommonFilters(r,now,age,search,origin)&&(!$('band').value||r.Band===$('band').value)).sort((a,b)=>Date.parse(b.ReceivedAtUTC)-Date.parse(a.ReceivedAtUTC)||b.EventID-a.EventID);
 const busiest=busiestBand(now,age,search,origin);$('activeband').textContent=busiest?`Most active now: ${busiest.band} (${busiest.count}) — subject to change`:'';
 const unique=new Set(visible.map(key)),unknown=visible.filter(r=>!r.DXLocation).length;$('count').textContent=unique.size+' stations / bands · '+unknown+' unlocated';page=Math.max(0,Math.min(page,Math.ceil(visible.length/50)-1));const pageRows=visible.slice(page*50,page*50+50),tbody=$('rows');tbody.replaceChildren();for(const r of pageRows){const row=document.createElement('tr');if(key(r)===selected)row.className='selected';for(const [i,value] of [new Date(r.ReceivedAtUTC).toISOString().slice(11,19),r.DXCall,(r.FrequencyHz/1e6).toFixed(4)+' / '+r.Band,r.SpotterCall,locationText(r.DXLocation),r.Comment].entries()){const td=document.createElement('td');if(i===1){const b=document.createElement('button');b.textContent=value;b.onclick=()=>select(key(r));td.append(b);}else td.textContent=value;row.append(td);}tbody.append(row);}
 $('listcount').textContent=visible.length?`${page*50+1}–${page*50+pageRows.length} of ${visible.length}`:'0 reports';$('prev').disabled=page===0;$('next').disabled=(page+1)*50>=visible.length;$('empty').hidden=visible.length>0;details();draw();if(focusRoot&&active.tagName==='BUTTON')[...$(focusRoot).querySelectorAll('button')].find(b=>b.textContent===focusText)?.focus({preventScroll:true});}
for(const id of ['band','age','filters','paths','origin','search'])$(id).addEventListener('input',()=>{page=0;remember();render();});
$('prev').onclick=()=>{page--;render();};$('next').onclick=()=>{page++;render();};
function zoom(f){scale=Math.max(1,Math.min(8,scale*f));if(scale===1)pan=[0,0];draw();}
$('zoomIn').onclick=()=>zoom(1.4);$('zoomOut').onclick=()=>zoom(1/1.4);$('reset').onclick=()=>{scale=1;pan=[0,0];draw();};$('fullscreen').onclick=()=>{const p=document.fullscreenElement?document.exitFullscreen():$('viewport').requestFullscreen();p?.catch(()=>{});};
canvas.addEventListener('wheel',e=>{e.preventDefault();zoom(e.deltaY<0?1.15:1/1.15);},{passive:false});
canvas.onpointerdown=e=>{drag={x:e.clientX,y:e.clientY,startX:e.clientX,startY:e.clientY,moved:false};canvas.setPointerCapture(e.pointerId);};canvas.onpointermove=e=>{if(!drag)return;pan[0]+=e.clientX-drag.x;pan[1]+=e.clientY-drag.y;drag.moved ||= Math.hypot(e.clientX-drag.startX,e.clientY-drag.startY)>4;drag.x=e.clientX;drag.y=e.clientY;draw();};canvas.onpointerup=e=>{if(drag&&!drag.moved){const box=canvas.getBoundingClientRect(),x=e.clientX-box.left,y=e.clientY-box.top;const hit=hits.find(p=>Math.hypot(p.x-x,p.y-y)<p.radius+6);if(hit){if(new Set(hit.group.map(key)).size>1)details(hit.group);else select(key(hit.group[0]));}}drag=null;};canvas.onpointercancel=()=>{drag=null;};
new ResizeObserver(draw).observe($('viewport'));
fetch('world.json').then(r=>{if(!r.ok)throw Error();return r.json();}).then(data=>{world=data;draw();}).catch(()=>{$('mapnotice').textContent='Could not load bundled world geography';});
// US state outlines are a supplementary layer; missing/failed load just
// means no state lines draw, not a map-wide error notice.
fetch('us_states.json').then(r=>{if(!r.ok)throw Error();return r.json();}).then(data=>{usStates=data;draw();}).catch(()=>{});
const stream=new EventSource('events');stream.onmessage=e=>{const packet=JSON.parse(e.data);online=true;lastPacket=Date.now();state=packet.State;if(packet.Reset)reports.clear();for(const r of packet.Reports)reports.set(r.EventID,r);for(const [id,r]of reports)if(id<packet.OldestID||Date.parse(r.ReceivedAtUTC)<Date.parse(packet.Now)-3600000)reports.delete(id);$('connection').textContent='● Logger connected';$('status').textContent=(state.Status||'Waiting for cluster')+(packet.AtCapacity?' · Report capacity reached; history may be truncated':'');$('home').textContent=state.Home?'★ '+state.Callsign+' · '+state.Home.Locator:'Home grid not configured';render();};stream.onerror=()=>{online=false;$('connection').textContent='Logger disconnected · reconnecting…';};
function tick(){$('clock').textContent=new Date().toISOString().slice(11,19)+' UTC';if(online&&Date.now()-lastPacket>5000)$('connection').textContent='Logger stream stale · waiting…';if(!online)render();}tick();setInterval(tick,1000);
