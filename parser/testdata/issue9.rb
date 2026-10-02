class BeforeIssue9
end

def to_s
  "Ruby DataSource"
end

"Some text (for #{arguments.size} arguments)" if arguments.size != 1

process_structclass(name, $')

log_creation_time_match = /
  (?<year>\d{4})-
  (?<month>\d{2})-
  (?<day>\d{2})_
  (?<hour>\d{2})
  (?<minute>\d{2})
  (?<second>\d{2})
  \d
  \.log
/x.match(log_path)

"nested #{ {key: "value"}.fetch(:key) }"

class AfterIssue9
  def indexed_after_literals
    "after"
  end
end
