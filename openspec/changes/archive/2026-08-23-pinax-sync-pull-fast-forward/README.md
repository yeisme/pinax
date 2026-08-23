# pinax-sync-pull-fast-forward

pull 在本地未偏离 base 时静默 fast-forward，不再为顺序编辑产生噪声冲突副本；统一 move 分支已有的 preserveConflict 规则并补 TOCTOU 内容防线。
