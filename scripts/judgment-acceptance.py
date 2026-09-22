#!/usr/bin/env python3
"""Owner-bound optional acceptance tool; never mutates canonical business state."""
import json, os, sys
from pathlib import Path
runtime=Path.home()/'.local/share/yeisme-judgment-acceptance'
sys.path.insert(0,str(runtime))
from acceptance_owner import main, case, cli_json, bounded
ROOT=Path(__file__).resolve().parents[3]
OWNER='pinax'
STORE=Path.home()/'.local/share'/OWNER/'judgment-acceptance/jev-real-20260921'

def collect():
 vault=ROOT/'data/yeisme-notes'
 inbox=cli_json(['pinax','inbox','list','--vault',str(vault),'--json'])['notes']
 notes=cli_json(['pinax','note','list','--vault',str(vault),'--limit','100','--json'])['notes']
 out=[];gaps=[]
 for note in inbox:
  candidates=[n for n in notes if n['id']!=note['id']]
  candidates.sort(key=lambda n:(-len(set(n.get('tags',[]))&set(note.get('tags',[]))),n['id']))
  if not candidates:continue
  other=candidates[0]
  try:
   excerpt='Inbox: '+bounded(note['title'],150)+'\n'+bounded(note.get('body',''),1300)+'\nCandidate note: '+bounded(other['title'],150)+'\n'+bounded(other.get('body',''),1300)
   out.append(case('pinax-note:'+note['id'],note['title'],excerpt,'原状态：inbox，笔记独立保存，未确认合并或链接。对照候选按已有标签交集选择，不是人工正确答案。',note['updated_at']+'|'+other['updated_at'],
    'Compare the inbox note with the existing candidate note. Is it a duplicate, complementary material worth linking, unrelated, or insufficient evidence? Do not treat shared keywords as proof of duplication.',
    {'duplicate':'Substantially duplicate content','complementary':'Distinct but complementary information worth linking','unrelated':'No useful relationship','insufficient':'Excerpts too incomplete to establish relationship'},['每条仅评估一个候选；不代表全 vault 已去重，不自动合并、删除或确认长期记忆。']))
  except ValueError:gaps.append('sensitive_source_excluded')
 return out,gaps

if __name__=="__main__":main(OWNER,STORE,collect)
