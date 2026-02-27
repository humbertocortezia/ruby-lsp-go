# Sample Rails Application Controller
# Demonstrates Ruby LSP Go capabilities

class ApplicationController < ActionController::Base
  protect_from_forgery with: :exception

  before_action :authenticate_user!
  
  # Method to demonstrate completion and hover
  def current_user
    @current_user ||= User.find(session[:user_id]) if session[:user_id]
  end
  
  # Another example method
  def admin_only
    redirect_to root_path unless current_user.admin?
  end
end

# Sample model to test LSP features
class User < ApplicationRecord
  has_many :posts
  has_many :comments
  
  validates :email, presence: true, uniqueness: true
  
  # Example instance method
  def full_name
    "#{first_name} #{last_name}"
  end
  
  # Method to check admin status
  def admin?
    role == "admin"
  end
end

# Sample helper module
module ApplicationHelper
  # Helper method for displaying user info
  def display_user_info(user)
    content_tag :div, class: "user-info" do
      concat content_tag(:span, user.full_name, class: "name")
      concat content_tag(:span, user.email, class: "email") if user.persisted?
    end
  end
end

