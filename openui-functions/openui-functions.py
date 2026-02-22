"""
title: LME RAG + Vault Write
author: Szymon Leja
"""

from typing import Optional, Dict, Any
import requests
import json


class Filter:
    class Valves:
        lme_url: str = "http://host.docker.internal:8080"
        lme_key: str = "demo123"
        topk: int = 3

    def __init__(self):
        self.valves = self.Valves()

    def inlet(
        self, body: Dict[str, Any], __user__: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """RAG: Inject LME chunks"""
        messages = body.get("messages", [])
        if not messages or messages[-1].get("role") != "user":
            return body

        query = messages[-1]["content"]
        resp = requests.post(
            f"{self.valves.lme_url}/query",
            json={"q": query, "topk": self.valves.topk},
            headers={
                "X-API-Key": self.valves.lme_key,
                "Content-Type": "application/json",
            },
            timeout=10,
        )

        if resp.status_code == 200:
            results = resp.json().get("results", [])
            
            # TOP 1 = PEŁNY plik, reszta chunki
            full_context = []
            if results:
                # Pobierz CAŁY top plik
                top_file = results[0]
                file_resp = requests.get(
                    f"{self.valves.lme_url}/file/{top_file.get('filename', top_file.get('file_path', ''))}",
                    headers={"X-API-Key": self.valves.lme_key},
                    timeout=5
                )
                if file_resp.status_code == 200:
                    full_context.append(f"📄 **{top_file.get('file_path')}**:\n{file_resp.json().get('content', '')[:1500]}")
            
            # Reszta jako chunki
            chunk_context = "\n".join([
                f"📄 **{r.get('file_path')}** [{r.get('chunk_index', 0)}]: {r.get('chunk_text', '')[:400]}"
                for r in results[1:self.valves.topk+1]
            ])
            
            context = "\n\n".join([c for c in [full_context[0] if full_context else "", chunk_context] if c])
            
            if context:
                messages.insert(-1, {
                    "role": "system",
                    "content": f"LME Vault ({len(results)} sources):\n{context}"
                })
                print(f"💾 RAG: 1 full file + {len(results)-1} chunks")
        
        return body

    def outlet(self, body: dict, __user__: Optional[dict] = None) -> dict:
        print("📝 OUTLET")

        messages = body.get("messages", [])
        if len(messages) < 2:
            return body

        # AI content
        content = ""
        for msg in reversed(messages):
            if msg.get("role") == "assistant":
                content = msg.get("content", "")
                break

        # User query → filename
        query = ""
        for msg in reversed(messages):
            if msg.get("role") == "user":
                query = msg["content"]
                break

        if not (content.strip() and query.strip()):
            return body

        # SMART FILENAME: pierwsze 3 słowa + .md
        filename_words = query.split()[:3]
        filename = "-".join(filename_words).lower() + ".md"
        print(f"📁 Filename: '{filename}' | Query: '{query[:50]}'")

        note = f"\n## Q: {query}\n\n{content}\n\n---\n"

        # 1. SPRÓBUJ PATCH append (jeśli istnieje)
        resp = requests.patch(
            f"{self.valves.lme_url}/ingest",
            json={"filename": filename, "append": note},  # DODAJ do końca!
            headers={
                "X-API-Key": self.valves.lme_key,
                "Content-Type": "application/json",
            },
            timeout=10,
        )

        if resp.status_code == 404:  # Nie istnieje → POST create
            print("📄 Create new file")
            resp = requests.post(
                f"{self.valves.lme_url}/ingest",
                json={
                    "filename": filename,
                    "content": f"# {filename.replace('-', ' ').title()}\n\n" + note,
                },
                headers={
                    "X-API-Key": self.valves.lme_key,
                    "Content-Type": "application/json",
                },
                timeout=10,
            )

        print(f"💾 {resp.status_code}: {resp.json().get('job_id')}")
        return body
