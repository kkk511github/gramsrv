UPDATE public.sticker_sets
SET hash = CASE
    WHEN hash >= 2147483647 THEN 1
    ELSE hash + 1
END
WHERE set_kind = 'emoji'
  AND COALESCE(creator_user_id, 0) = 0;
