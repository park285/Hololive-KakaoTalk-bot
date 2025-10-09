#!/usr/bin/env python3
"""
홀로라이브 멤버 프로필 JSON 파일의 일본어 레이블을 한국어로 변환하는 스크립트
"""
import json
import os
from pathlib import Path

def fix_hashtag_labels(value: str) -> str:
    """해시태그 value 내의 일본어 레이블을 한국어로 변환"""
    if not isinstance(value, str):
        return value

    # 일본어 레이블을 한국어로 변환
    replacements = {
        '配信タグ：': '방송 태그: ',
        '配信タグ:': '방송 태그: ',
        '配信タグ': '방송 태그: ',
        'ファンアートタグ：': '팬아트 태그: ',
        'ファンアートタグ:': '팬아트 태그: ',
        'ファンアート：': '팬아트 태그: ',
        'ファンアート:': '팬아트 태그: ',
        'ファンアート': '팬아트 태그: ',
        'アートタグ：': '팬아트 태그: ',
        'アートタグ :': '팬아트 태그: ',
        'アートタグ': '팬아트 태그: ',
        '切り抜き動画：': '클립: ',
        '切り抜き動画:': '클립: ',
        '切り抜き：': '클립: ',
        '切り抜き:': '클립: ',
        'ミーム投稿：': '밈: ',
        'ミーム投稿:': '밈: ',
        'ミーム：': '밈: ',
        'ミーム:': '밈: ',
        '楽曲感想：': '악곡 감상: ',
        '楽曲感想:': '악곡 감상: ',
        '歌の感想：': '악곡 감상: ',
        '歌の感想:': '악곡 감상: ',
        'ボイス感想：': '보이스 감상: ',
        'ボイス感想:': '보이스 감상: ',
        'ショート：': '쇼트: ',
        'ショート:': '쇼트: ',
        'MMD作品：': 'MMD 작품: ',
        'MMD作品:': 'MMD 작품: ',
        'マイクラ投稿：': '마인크래프트: ',
        'マイクラ投稿:': '마인크래프트: ',
        'ホロリー＆聖地巡礼：': '홀로리 & 성지순례: ',
        'ホロリー＆聖地巡礼:': '홀로리 & 성지순례: ',
        'ファンネーム：': '팬 이름: ',
        'ファンネーム:': '팬 이름: ',
        '정기방송：': '정기방송: ',
        '밈：': '밈: ',
    }

    result = value
    for jp_label, kr_label in replacements.items():
        result = result.replace(jp_label, kr_label)

    # 중복 콜론 제거: "방송 태그:  : " → "방송 태그: "
    import re
    result = re.sub(r':\s*:\s*', ': ', result)

    return result

def process_json_file(file_path: Path) -> dict:
    """JSON 파일을 읽어서 수정하고 결과 반환"""
    with open(file_path, 'r', encoding='utf-8') as f:
        data = json.load(f)

    modified = False

    # data 배열 순회
    if 'data' in data and isinstance(data['data'], list):
        for item in data['data']:
            if not isinstance(item, dict):
                continue

            label = item.get('label', '')
            value = item.get('value', '')

            # 해시태그 관련 필드 수정
            if label in ['해시태그', '방송 해시태그', '방송 태그', '팬아트 태그', '팬아트', '오시마크']:
                new_value = fix_hashtag_labels(value)
                if new_value != value:
                    item['value'] = new_value
                    modified = True

    return {'data': data, 'modified': modified}

def main():
    # 프로필 디렉토리 경로
    profiles_dir = Path(__file__).parent.parent / 'internal' / 'domain' / 'data' / 'official_profiles_ko'

    if not profiles_dir.exists():
        print(f"❌ 디렉토리를 찾을 수 없습니다: {profiles_dir}")
        return

    print(f"📂 작업 디렉토리: {profiles_dir}")
    print("=" * 80)

    modified_files = []
    error_files = []

    # 모든 JSON 파일 처리
    for json_file in sorted(profiles_dir.glob('*.json')):
        try:
            result = process_json_file(json_file)

            if result['modified']:
                # 수정된 내용을 파일에 저장 (한국어 유지)
                with open(json_file, 'w', encoding='utf-8') as f:
                    json.dump(result['data'], f, ensure_ascii=False, indent=2)

                modified_files.append(json_file.name)
                print(f"✅ 수정 완료: {json_file.name}")
            else:
                print(f"⏭️  변경 없음: {json_file.name}")

        except Exception as e:
            error_files.append((json_file.name, str(e)))
            print(f"❌ 오류 발생: {json_file.name} - {e}")

    # 결과 요약
    print("=" * 80)
    print(f"\n📊 작업 결과:")
    print(f"   • 수정된 파일: {len(modified_files)}개")
    print(f"   • 오류 발생: {len(error_files)}개")

    if modified_files:
        print(f"\n✨ 수정된 파일 목록:")
        for filename in modified_files:
            print(f"   - {filename}")

    if error_files:
        print(f"\n⚠️ 오류 발생 파일:")
        for filename, error in error_files:
            print(f"   - {filename}: {error}")

if __name__ == '__main__':
    main()
