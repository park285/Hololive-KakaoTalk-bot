#!/usr/bin/env python3
"""일본어 해시태그 라벨을 한국어로 치환"""

import json
import os
from pathlib import Path

# 치환 매핑
LABEL_REPLACEMENTS = {
    "配信タグ": "방송 태그",
    "ファンアート": "팬아트",
    "歌の感想": "노래 감상",
    "ミーム投稿": "밈 투고",
    "ミーム": "밈",
    "マイクラ投稿": "마인크래프트 투고",
    "切り抜き": "클립",
    "切り抜き動画": "클립 영상",
}

def fix_hashtag_labels(value: str) -> str:
    """해시태그 값 안의 일본어 라벨 치환"""
    result = value
    for jp, ko in LABEL_REPLACEMENTS.items():
        # コロン 포함해서 치환
        result = result.replace(f"{jp}：", f"{ko}: ")
        result = result.replace(f"{jp}:", f"{ko}: ")
    return result

def process_file(file_path: Path):
    """단일 JSON 파일 처리"""
    with open(file_path, 'r', encoding='utf-8') as f:
        data = json.load(f)

    modified = False

    # data 배열 처리
    if 'data' in data and isinstance(data['data'], list):
        for item in data['data']:
            if 'value' in item and isinstance(item['value'], str):
                original = item['value']
                fixed = fix_hashtag_labels(original)
                if original != fixed:
                    item['value'] = fixed
                    modified = True

    if modified:
        # 파일 업데이트
        with open(file_path, 'w', encoding='utf-8') as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
        return True
    return False

def main():
    """메인 실행"""
    profiles_dir = Path("internal/domain/data/official_profiles_ko")

    if not profiles_dir.exists():
        print(f"Error: {profiles_dir} not found")
        return

    print("일본어 라벨 치환 시작...")
    print("="*60)

    total = 0
    fixed = 0

    for json_file in profiles_dir.glob("*.json"):
        total += 1
        if process_file(json_file):
            fixed += 1
            print(f"✅ {json_file.stem}")

    print("="*60)
    print(f"완료: {fixed}/{total} 파일 수정됨")

    # 통합 JSON도 처리
    combined_file = Path("internal/domain/data/official_profiles_ko.json")
    if combined_file.exists():
        print("\n통합 JSON 처리 중...")
        with open(combined_file, 'r', encoding='utf-8') as f:
            all_data = json.load(f)

        modified_count = 0
        for slug, profile in all_data.items():
            if 'data' in profile and isinstance(profile['data'], list):
                for item in profile['data']:
                    if 'value' in item and isinstance(item['value'], str):
                        original = item['value']
                        fixed_val = fix_hashtag_labels(original)
                        if original != fixed_val:
                            item['value'] = fixed_val
                            modified_count += 1

        if modified_count > 0:
            with open(combined_file, 'w', encoding='utf-8') as f:
                json.dump(all_data, f, ensure_ascii=False, indent=2)
            print(f"✅ 통합 JSON 수정됨 ({modified_count} 항목)")

if __name__ == "__main__":
    main()
